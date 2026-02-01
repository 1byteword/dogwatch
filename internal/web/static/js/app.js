'use strict';

/**
 * dogwatch - eBPF Observability Dashboard
 * Main application script
 */

// App state - all globals declared upfront to avoid hoisting issues
let grid = null;
let cpuChart = null;
let memChart = null;
let currentUser = null;
let currentOrg = null;
let currentDashboardId = null;
let dashboards = [];
let svcMapData = { nodes: [], links: [] };
let svcMapLayout = 'hierarchical';
let svcMapFilter = '';
let svcMapSim = null;
let svcMapZoomBehavior = null;
let svcMapTransform = null; // Will be set to d3.zoomIdentity when D3 loads
let svcMapSelectedNode = null;
let svcMapMainGroup = null;
let selectedTraceId = null;
let traceServices = [];
let currentTraceData = null;
let selectedSpanId = null;
let traceServiceFilter = null;
let flameData = null;
let flameBaselineData = null;
let flameSearchTerm = '';
let flameZoomStack = [];
let flameMode = 'flame';
let flameDiffMode = false;
let flameTotalSamples = 0;
let watchMetrics = [];
let channels = [];
let logTailInterval = null;
let logTailEnabled = false;
let customLogTimeStart = null;
let customLogTimeEnd = null;
let syntheticChecks = [];
let statusPages = [];
let statusComponents = [];
let catalogServices = [];
let catalogTeams = [];
let correlationData = { correlations: [], timeline: null };
let currentK8sTab = 'pods';
let dbwatchCurrentTab = 'queries';

// DemoData placeholder - full definition is below, using var for hoisting
var DemoData = { enabled: false };

// Chart configuration
const chartOpts = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    plugins: { legend: { display: false } },
    scales: {
        x: {
            type: 'time',
            grid: { color: '#2f3336' },
            ticks: { color: '#71767b', font: { size: 9 }, maxTicksLimit: 6 }
        },
        y: {
            grid: { color: '#2f3336' },
            ticks: { color: '#71767b', font: { size: 9 } },
            beginAtZero: true,
            max: 100
        }
    }
};

// Smoothly hide the loading skeleton
function hideSkeleton() {
    const skeleton = document.getElementById('loading-skeleton');
    if (skeleton) {
        skeleton.classList.add('hidden');
        setTimeout(() => skeleton.style.display = 'none', 200);
    }
}

// Initialize app when DOM is ready
document.addEventListener('DOMContentLoaded', async () => {
    try {
        // Check auth first
        const isAuthenticated = await checkAuth();

        if (isAuthenticated) {
            // Load dashboard dependencies (GridStack, Charts)
            await window.initDashboard();

            // Hide skeleton with animation
            hideSkeleton();

            // Initialize grid and show app
            initGrid();
            loadLayout();

            // Expose grid and functions for widget picker
            window.grid = grid;
            window.getWidgetContent = getWidgetContent;
            window.saveLayout = saveLayout;

            // Wait for GridStack to finish rendering before loading data
            requestAnimationFrame(() => {
                startDataRefresh();
            });

            // Reveal app with smooth fade-in
            const app = document.getElementById('app');
            if (app) {
                requestAnimationFrame(() => {
                    app.classList.add('loaded');
                });
            }
        }
    } catch (err) {
        console.error('App initialization failed:', err);
        const skeleton = document.getElementById('loading-skeleton');
        if (skeleton) {
            skeleton.innerHTML = `
                <div class="skeleton-logo">&#128021;</div>
                <div class="skeleton-text" style="color: var(--error)">Failed to load</div>
                <button class="btn btn-primary" onclick="location.reload()">Retry</button>
            `;
        }
    }
});

// =====================================
// Authentication & User Management
// =====================================
// moved to top
// moved to top

// Check authentication on page load
async function checkAuth() {
    try {
        const resp = await fetch('/api/auth/me');
        if (resp.ok) {
            const data = await resp.json();
            currentUser = data.user;
            currentOrg = data.org;
            showAuthenticatedUI();
            return true;
        }
    } catch (e) {}
    showLoginScreen();
    return false;
}


function showLoginScreen() {
    // Hide skeleton with animation
    hideSkeleton();

    // Show login screen
    const loginScreen = document.getElementById('login-screen');
    if (loginScreen) loginScreen.classList.add('show');
    
    // Load OAuth providers
    loadOAuthProviders();
}

// OAuth/SSO functions
async function loadOAuthProviders() {
    const oauthButtons = document.getElementById('oauth-buttons');
    const oauthLoading = document.getElementById('oauth-loading');
    const loginDivider = document.getElementById('login-divider');

    oauthLoading.classList.add('show');

    try {
        const resp = await fetch('/api/auth/providers');
        if (resp.ok) {
            const data = await resp.json();
            const providers = data.providers || [];
            const oauthProviders = providers.filter(p => p.type === 'oauth2');

            if (oauthProviders.length > 0) {
                // Show/hide buttons based on available providers
                document.querySelector('.oauth-btn.google').style.display =
                    oauthProviders.some(p => p.id === 'google') ? 'flex' : 'none';
                document.querySelector('.oauth-btn.github').style.display =
                    oauthProviders.some(p => p.id === 'github') ? 'flex' : 'none';
                document.querySelector('.oauth-btn.microsoft').style.display =
                    oauthProviders.some(p => p.id === 'microsoft') ? 'flex' : 'none';

                oauthButtons.style.display = 'flex';
                loginDivider.style.display = 'flex';
            }
        }
    } catch (e) {
        console.log('No OAuth providers available');
    }

    oauthLoading.classList.remove('show');
}

function startOAuth(provider) {
    // Redirect to OAuth start endpoint
    window.location.href = '/api/auth/oauth/' + provider;
}

// Check for OAuth callback success (token in URL or cookie)
function checkOAuthCallback() {
    const urlParams = new URLSearchParams(window.location.search);
    const error = urlParams.get('error');

    if (error) {
        const errorEl = document.getElementById('login-error');
        errorEl.textContent = 'OAuth failed: ' + (urlParams.get('error_description') || error);
        errorEl.classList.add('show');
        // Clear URL params
        window.history.replaceState({}, document.title, window.location.pathname);
        return false;
    }

    return false;
}

// Token refresh functionality
async function refreshAccessToken() {
    try {
        const resp = await fetch('/api/auth/token/refresh', {
            method: 'POST',
            credentials: 'include'
        });

        if (resp.ok) {
            const data = await resp.json();
            // Token refreshed via cookie
            return true;
        }
    } catch (e) {
        console.log('Token refresh failed');
    }
    return false;
}

// Set up token refresh interval (every 10 minutes)
setInterval(async () => {
    if (currentUser) {
        await refreshAccessToken();
    }
}, 10 * 60 * 1000);


function showAuthenticatedUI() {
    // Hide login screen
    const loginScreen = document.getElementById('login-screen');
    if (loginScreen) loginScreen.classList.remove('show');
    
    // Show user menu
    document.getElementById('user-menu').style.display = 'block';
    
    // Update user info in header
    if (currentUser) {
        const initials = (currentUser.name || currentUser.email || '?').substring(0, 2).toUpperCase();
        document.getElementById('user-avatar').textContent = initials;
        document.getElementById('user-name-display').textContent = currentUser.name || currentUser.email;
        document.getElementById('user-full-name').textContent = currentUser.name || 'User';
        document.getElementById('user-email').textContent = currentUser.email || '';
        document.getElementById('user-role-badge').textContent = currentUser.role || 'viewer';
        document.getElementById('user-role-badge').className = 'user-menu-role ' + (currentUser.role || '');
        
        // Show admin menu items
        if (currentUser.role === 'owner' || currentUser.role === 'admin') {
            const usersItem = document.getElementById('users-menu-item');
            const teamsItem = document.getElementById('teams-menu-item');
            if (usersItem) usersItem.style.display = 'flex';
            if (teamsItem) teamsItem.style.display = 'flex';
        }
    }
}

function toggleUserMenu() {
    const dropdown = document.getElementById('user-menu-dropdown');
    dropdown.classList.toggle('show');
}

// Close menu when clicking outside
document.addEventListener('click', function(e) {
    const userMenu = document.getElementById('user-menu');
    if (userMenu && !userMenu.contains(e.target)) {
        document.getElementById('user-menu-dropdown')?.classList.remove('show');
    }
});

async function handleLogin(event) {
    event.preventDefault();
    const email = document.getElementById('login-email').value;
    const password = document.getElementById('login-password').value;
    const errorEl = document.getElementById('login-error');
    errorEl.classList.remove('show');

    try {
        const resp = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, password })
        });

        if (resp.ok) {
            const data = await resp.json();
            currentUser = data.user;
            showAuthenticatedUI();
            // Re-initialize the dashboard
            loadLayout();
            loadDashboards();
            setTimeout(initCharts, 100);
        } else {
            errorEl.textContent = 'Invalid email or password';
            errorEl.classList.add('show');
        }
    } catch (e) {
        errorEl.textContent = 'Connection error. Please try again.';
        errorEl.classList.add('show');
    }
}

async function handleLogout() {
    try {
        await fetch('/api/auth/logout', { method: 'POST' });
    } catch (e) {}
    currentUser = null;
    currentOrg = null;
    showLoginScreen();
}

function showUserSettings() {
    document.getElementById('user-menu-dropdown').classList.remove('show');
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">User Settings</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Name</label>
                    <input type="text" id="settings-name" class="form-input" value="${escapeHtml(currentUser?.name || '')}">
                </div>
                <div class="form-group">
                    <label class="form-label">Email</label>
                    <input type="email" id="settings-email" class="form-input" value="${escapeHtml(currentUser?.email || '')}" readonly>
                </div>
                <div class="form-group">
                    <label class="form-label">Timezone</label>
                    <select id="settings-timezone" class="form-select">
                        <option value="">Auto-detect</option>
                        <option value="UTC">UTC</option>
                        <option value="America/New_York">Eastern Time</option>
                        <option value="America/Chicago">Central Time</option>
                        <option value="America/Denver">Mountain Time</option>
                        <option value="America/Los_Angeles">Pacific Time</option>
                        <option value="Europe/London">London</option>
                        <option value="Europe/Paris">Paris</option>
                        <option value="Asia/Tokyo">Tokyo</option>
                    </select>
                </div>
                <hr style="border-color: #2f3336; margin: 1rem 0;">
                <h4 style="font-size: 0.85rem; margin-bottom: 0.8rem;">Change Password</h4>
                <div class="form-group">
                    <label class="form-label">Current Password</label>
                    <input type="password" id="settings-old-password" class="form-input">
                </div>
                <div class="form-group">
                    <label class="form-label">New Password</label>
                    <input type="password" id="settings-new-password" class="form-input">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="saveUserSettings()">Save Changes</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
    // Set current timezone
    if (currentUser?.timezone) {
        document.getElementById('settings-timezone').value = currentUser.timezone;
    }
}

async function saveUserSettings() {
    const name = document.getElementById('settings-name').value;
    const timezone = document.getElementById('settings-timezone').value;
    const oldPassword = document.getElementById('settings-old-password').value;
    const newPassword = document.getElementById('settings-new-password').value;

    try {
        // Update user info
        const resp = await fetch(`/api/users/${currentUser.id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, timezone })
        });
        if (resp.ok) {
            const updatedUser = await resp.json();
            currentUser = updatedUser;
            showAuthenticatedUI();
        }

        // Change password if provided
        if (oldPassword && newPassword) {
            const pwResp = await fetch('/api/auth/password', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ old_password: oldPassword, new_password: newPassword })
            });
            if (!pwResp.ok) {
                alert('Failed to change password. Check your current password.');
                return;
            }
        }

        document.querySelector('.modal-overlay')?.remove();
    } catch (e) {
        alert('Failed to save settings');
    }
}

// =====================================
// User Management (Admin)
// =====================================
async function showUserManagement() {
    document.getElementById('user-menu-dropdown').classList.remove('show');
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width: 600px;">
            <div class="modal-header">
                <span class="modal-title">User Management</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body" style="padding: 0;">
                <div style="padding: 0.8rem; border-bottom: 1px solid #2f3336; display: flex; justify-content: space-between; align-items: center;">
                    <span style="color: #71767b; font-size: 0.8rem;" id="users-org-name">${escapeHtml(currentOrg?.name || 'Organization')}</span>
                    <button class="btn btn-primary" onclick="showInviteUserModal()">+ Invite User</button>
                </div>
                <div class="users-list" id="users-list">
                    <div class="empty-state">Loading users...</div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="showPendingInvites()">Pending Invites</button>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
    loadUsersList();
}

async function loadUsersList() {
    try {
        const resp = await fetch('/api/users');
        const users = await resp.json();
        const list = document.getElementById('users-list');
        if (!users?.length) {
            list.innerHTML = '<div class="empty-state">No users found</div>';
            return;
        }
        list.innerHTML = users.map(u => `
            <div class="user-item ${u.is_active ? '' : 'user-inactive'}">
                <div class="user-item-avatar">${(u.name || u.email).substring(0, 2).toUpperCase()}</div>
                <div class="user-item-info">
                    <div class="user-item-name">
                        ${escapeHtml(u.name || 'Unnamed')}
                        ${u.id === currentUser?.id ? '<span style="color: #1d9bf0; font-size: 0.7rem;">(you)</span>' : ''}
                        ${!u.is_active ? '<span style="color: #f4212e; font-size: 0.7rem;">(inactive)</span>' : ''}
                    </div>
                    <div class="user-item-email">${escapeHtml(u.email)}</div>
                </div>
                <span class="user-item-role ${u.role}">${u.role}</span>
                <div class="user-item-actions">
                    ${u.id !== currentUser?.id ? `
                        <button class="btn" onclick="editUser('${u.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem;">Edit</button>
                        <button class="btn" onclick="toggleUserActive('${u.id}', ${u.is_active})" style="padding: 0.2rem 0.4rem; font-size: 0.65rem;">${u.is_active ? 'Disable' : 'Enable'}</button>
                    ` : ''}
                </div>
            </div>
        `).join('');
    } catch (e) {
        document.getElementById('users-list').innerHTML = '<div class="empty-state">Failed to load users</div>';
    }
}

function showInviteUserModal() {
    const inviteModal = document.createElement('div');
    inviteModal.className = 'modal-overlay';
    inviteModal.style.zIndex = '1001';
    inviteModal.onclick = (e) => { if (e.target === inviteModal) inviteModal.remove(); };
    inviteModal.innerHTML = `
        <div class="modal" style="max-width: 400px;">
            <div class="modal-header">
                <span class="modal-title">Invite User</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Email Address</label>
                    <input type="email" id="invite-email" class="form-input" placeholder="user@example.com">
                </div>
                <div class="form-group">
                    <label class="form-label">Role</label>
                    <select id="invite-role" class="form-select">
                        <option value="viewer">Viewer</option>
                        <option value="editor">Editor</option>
                        ${currentUser?.role === 'owner' ? '<option value="admin">Admin</option>' : ''}
                    </select>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="sendInvite()">Send Invite</button>
            </div>
        </div>
    `;
    document.body.appendChild(inviteModal);
}

async function sendInvite() {
    const email = document.getElementById('invite-email').value;
    const role = document.getElementById('invite-role').value;
    if (!email) { alert('Email is required'); return; }

    try {
        const resp = await fetch('/api/auth/invites', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email, role })
        });
        if (resp.ok) {
            const data = await resp.json();
            // Show the invite token (in a real app, this would be sent via email)
            alert('Invitation created! Token: ' + data.token + '\\n\\nShare this with the user to accept the invite.');
            document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
            loadUsersList();
        } else {
            alert('Failed to create invitation');
        }
    } catch (e) {
        alert('Failed to create invitation');
    }
}

async function showPendingInvites() {
    try {
        const resp = await fetch('/api/auth/invites');
        const invites = await resp.json();

        const inviteModal = document.createElement('div');
        inviteModal.className = 'modal-overlay';
        inviteModal.style.zIndex = '1001';
        inviteModal.onclick = (e) => { if (e.target === inviteModal) inviteModal.remove(); };
        inviteModal.innerHTML = `
            <div class="modal" style="max-width: 500px;">
                <div class="modal-header">
                    <span class="modal-title">Pending Invitations</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body" style="max-height: 300px; overflow-y: auto;">
                    ${!invites?.length ? '<div class="empty-state">No pending invitations</div>' : invites.map(inv => `
                        <div style="padding: 0.6rem 0; border-bottom: 1px solid #2f3336; display: flex; justify-content: space-between; align-items: center;">
                            <div>
                                <div style="font-weight: 500;">${escapeHtml(inv.email)}</div>
                                <div style="font-size: 0.7rem; color: #71767b;">Role: ${inv.role} | Expires: ${new Date(inv.expires_at).toLocaleDateString()}</div>
                            </div>
                            <button class="btn" style="background: #4a1919; color: #f4212e; padding: 0.2rem 0.4rem; font-size: 0.65rem;" onclick="deleteInvite('${inv.id}')">Revoke</button>
                        </div>
                    `).join('')}
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                </div>
            </div>
        `;
        document.body.appendChild(inviteModal);
    } catch (e) {
        alert('Failed to load invitations');
    }
}

async function deleteInvite(id) {
    try {
        await fetch(`/api/auth/invites/${id}`, { method: 'DELETE' });
        document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
        showPendingInvites();
    } catch (e) {}
}

async function editUser(userId) {
    try {
        const resp = await fetch(`/api/users/${userId}`);
        const user = await resp.json();

        const editModal = document.createElement('div');
        editModal.className = 'modal-overlay';
        editModal.style.zIndex = '1001';
        editModal.onclick = (e) => { if (e.target === editModal) editModal.remove(); };
        editModal.innerHTML = `
            <div class="modal" style="max-width: 400px;">
                <div class="modal-header">
                    <span class="modal-title">Edit User</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group">
                        <label class="form-label">Name</label>
                        <input type="text" id="edit-user-name" class="form-input" value="${escapeHtml(user.name || '')}">
                    </div>
                    <div class="form-group">
                        <label class="form-label">Role</label>
                        <select id="edit-user-role" class="form-select">
                            <option value="viewer" ${user.role === 'viewer' ? 'selected' : ''}>Viewer</option>
                            <option value="editor" ${user.role === 'editor' ? 'selected' : ''}>Editor</option>
                            ${currentUser?.role === 'owner' ? `<option value="admin" ${user.role === 'admin' ? 'selected' : ''}>Admin</option>` : ''}
                        </select>
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn" style="background: #4a1919; color: #f4212e;" onclick="deleteUser('${userId}')">Delete User</button>
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                    <button class="btn btn-primary" onclick="saveUserEdit('${userId}')">Save</button>
                </div>
            </div>
        `;
        document.body.appendChild(editModal);
    } catch (e) {
        alert('Failed to load user');
    }
}

async function saveUserEdit(userId) {
    const name = document.getElementById('edit-user-name').value;
    const role = document.getElementById('edit-user-role').value;

    try {
        await fetch(`/api/users/${userId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, role })
        });
        document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
        loadUsersList();
    } catch (e) {
        alert('Failed to update user');
    }
}

async function toggleUserActive(userId, isActive) {
    try {
        await fetch(`/api/users/${userId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ is_active: !isActive })
        });
        loadUsersList();
    } catch (e) {
        alert('Failed to update user');
    }
}

async function deleteUser(userId) {
    if (!confirm('Are you sure you want to delete this user?')) return;
    try {
        await fetch(`/api/users/${userId}`, { method: 'DELETE' });
        document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
        loadUsersList();
    } catch (e) {
        alert('Failed to delete user');
    }
}

// =====================================
// Team Management
// =====================================
async function showTeamManagement() {
    document.getElementById('user-menu-dropdown').classList.remove('show');
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width: 600px;">
            <div class="modal-header">
                <span class="modal-title">Teams</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body" style="padding: 0;">
                <div style="padding: 0.8rem; border-bottom: 1px solid #2f3336; display: flex; justify-content: flex-end;">
                    <button class="btn btn-primary" onclick="showCreateTeamModal()">+ Create Team</button>
                </div>
                <div class="users-list" id="teams-list">
                    <div class="empty-state">Loading teams...</div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
    loadTeamsList();
}

async function loadTeamsList() {
    try {
        const resp = await fetch('/api/teams');
        const teams = await resp.json();
        const list = document.getElementById('teams-list');
        if (!teams?.length) {
            list.innerHTML = '<div class="empty-state">No teams found</div>';
            return;
        }
        list.innerHTML = teams.map(t => `
            <div class="user-item">
                <div class="user-item-avatar" style="background: #7c3aed;">${(t.name || '?').substring(0, 2).toUpperCase()}</div>
                <div class="user-item-info">
                    <div class="user-item-name">${escapeHtml(t.name)}</div>
                    <div class="user-item-email">${t.member_ids?.length || 0} members</div>
                </div>
                <div class="user-item-actions">
                    <button class="btn" onclick="editTeam('${t.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem;">Edit</button>
                    <button class="btn" onclick="deleteTeam('${t.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem; background: #4a1919; color: #f4212e;">Delete</button>
                </div>
            </div>
        `).join('');
    } catch (e) {
        document.getElementById('teams-list').innerHTML = '<div class="empty-state">Failed to load teams</div>';
    }
}

function showCreateTeamModal() {
    const createModal = document.createElement('div');
    createModal.className = 'modal-overlay';
    createModal.style.zIndex = '1001';
    createModal.onclick = (e) => { if (e.target === createModal) createModal.remove(); };
    createModal.innerHTML = `
        <div class="modal" style="max-width: 400px;">
            <div class="modal-header">
                <span class="modal-title">Create Team</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Team Name</label>
                    <input type="text" id="team-name" class="form-input" placeholder="Platform Team">
                </div>
                <div class="form-group">
                    <label class="form-label">Description</label>
                    <input type="text" id="team-description" class="form-input" placeholder="Team description">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createTeam()">Create Team</button>
            </div>
        </div>
    `;
    document.body.appendChild(createModal);
}

async function createTeam() {
    const name = document.getElementById('team-name').value;
    const description = document.getElementById('team-description').value;
    if (!name) { alert('Team name is required'); return; }

    try {
        await fetch('/api/teams', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, description, member_ids: [] })
        });
        document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
        loadTeamsList();
    } catch (e) {
        alert('Failed to create team');
    }
}

async function editTeam(teamId) {
    // For simplicity, just show team name edit. Full member management could be added.
    try {
        const resp = await fetch(`/api/teams/${teamId}`);
        const team = await resp.json();

        const editModal = document.createElement('div');
        editModal.className = 'modal-overlay';
        editModal.style.zIndex = '1001';
        editModal.onclick = (e) => { if (e.target === editModal) editModal.remove(); };
        editModal.innerHTML = `
            <div class="modal" style="max-width: 400px;">
                <div class="modal-header">
                    <span class="modal-title">Edit Team</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group">
                        <label class="form-label">Team Name</label>
                        <input type="text" id="edit-team-name" class="form-input" value="${escapeHtml(team.name || '')}">
                    </div>
                    <div class="form-group">
                        <label class="form-label">Description</label>
                        <input type="text" id="edit-team-description" class="form-input" value="${escapeHtml(team.description || '')}">
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                    <button class="btn btn-primary" onclick="saveTeamEdit('${teamId}')">Save</button>
                </div>
            </div>
        `;
        document.body.appendChild(editModal);
    } catch (e) {
        alert('Failed to load team');
    }
}

async function saveTeamEdit(teamId) {
    const name = document.getElementById('edit-team-name').value;
    const description = document.getElementById('edit-team-description').value;

    try {
        await fetch(`/api/teams/${teamId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, description })
        });
        document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
        loadTeamsList();
    } catch (e) {
        alert('Failed to update team');
    }
}

async function deleteTeam(teamId) {
    if (!confirm('Are you sure you want to delete this team?')) return;
    try {
        await fetch(`/api/teams/${teamId}`, { method: 'DELETE' });
        loadTeamsList();
    } catch (e) {
        alert('Failed to delete team');
    }
}

// =====================================
// API Key Management
// =====================================
async function showAPIKeyManagement() {
    document.getElementById('user-menu-dropdown').classList.remove('show');
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width: 600px;">
            <div class="modal-header">
                <span class="modal-title">API Keys</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body" style="padding: 0;">
                <div style="padding: 0.8rem; border-bottom: 1px solid #2f3336; display: flex; justify-content: flex-end;">
                    <button class="btn btn-primary" onclick="showCreateAPIKeyModal()">+ Create API Key</button>
                </div>
                <div id="apikeys-list" style="max-height: 400px; overflow-y: auto;">
                    <div class="empty-state">Loading API keys...</div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
    loadAPIKeysList();
}

async function loadAPIKeysList() {
    try {
        const resp = await fetch('/api/apikeys');
        const keys = await resp.json();
        const list = document.getElementById('apikeys-list');
        if (!keys?.length) {
            list.innerHTML = '<div class="empty-state">No API keys</div>';
            return;
        }
        list.innerHTML = keys.map(k => `
            <div class="apikey-item">
                <div class="apikey-header">
                    <div>
                        <span class="apikey-name">${escapeHtml(k.name)}</span>
                        <span class="apikey-prefix">${escapeHtml(k.key_prefix)}...</span>
                    </div>
                    <button class="btn" onclick="deleteAPIKey('${k.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem; background: #4a1919; color: #f4212e;">Delete</button>
                </div>
                <div class="apikey-meta">
                    <span>Created: ${new Date(k.created_at).toLocaleDateString()}</span>
                    ${k.last_used_at ? `<span>Last used: ${new Date(k.last_used_at).toLocaleDateString()}</span>` : '<span>Never used</span>'}
                    ${k.expires_at ? `<span>Expires: ${new Date(k.expires_at).toLocaleDateString()}</span>` : '<span>No expiry</span>'}
                </div>
            </div>
        `).join('');
    } catch (e) {
        document.getElementById('apikeys-list').innerHTML = '<div class="empty-state">Failed to load API keys</div>';
    }
}

function showCreateAPIKeyModal() {
    const createModal = document.createElement('div');
    createModal.className = 'modal-overlay';
    createModal.style.zIndex = '1001';
    createModal.onclick = (e) => { if (e.target === createModal) createModal.remove(); };
    createModal.innerHTML = `
        <div class="modal" style="max-width: 400px;">
            <div class="modal-header">
                <span class="modal-title">Create API Key</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Name</label>
                    <input type="text" id="apikey-name" class="form-input" placeholder="CI/CD Pipeline">
                </div>
                <div class="form-group">
                    <label class="form-label">Expires In</label>
                    <select id="apikey-expires" class="form-select">
                        <option value="30d">30 days</option>
                        <option value="90d">90 days</option>
                        <option value="1y">1 year</option>
                        <option value="never">Never</option>
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label">Permissions</label>
                    <div style="font-size: 0.75rem; color: #71767b; margin-bottom: 0.5rem;">Select resources this key can access:</div>
                    <div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 0.5rem;">
                        <label style="display: flex; align-items: center; gap: 0.3rem; font-size: 0.75rem;">
                            <input type="checkbox" id="perm-all" onchange="toggleAllPerms(this.checked)"> All access
                        </label>
                        <label style="display: flex; align-items: center; gap: 0.3rem; font-size: 0.75rem;">
                            <input type="checkbox" class="perm-check" value="metrics"> Metrics
                        </label>
                        <label style="display: flex; align-items: center; gap: 0.3rem; font-size: 0.75rem;">
                            <input type="checkbox" class="perm-check" value="logs"> Logs
                        </label>
                        <label style="display: flex; align-items: center; gap: 0.3rem; font-size: 0.75rem;">
                            <input type="checkbox" class="perm-check" value="traces"> Traces
                        </label>
                        <label style="display: flex; align-items: center; gap: 0.3rem; font-size: 0.75rem;">
                            <input type="checkbox" class="perm-check" value="alerts"> Alerts
                        </label>
                        <label style="display: flex; align-items: center; gap: 0.3rem; font-size: 0.75rem;">
                            <input type="checkbox" class="perm-check" value="dashboards"> Dashboards
                        </label>
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createAPIKey()">Create Key</button>
            </div>
        </div>
    `;
    document.body.appendChild(createModal);
}

function toggleAllPerms(checked) {
    document.querySelectorAll('.perm-check').forEach(cb => {
        cb.checked = checked;
        cb.disabled = checked;
    });
}

async function createAPIKey() {
    const name = document.getElementById('apikey-name').value;
    const expiresIn = document.getElementById('apikey-expires').value;
    if (!name) { alert('Name is required'); return; }

    const allAccess = document.getElementById('perm-all').checked;
    const permissions = [];
    if (allAccess) {
        permissions.push({ resource: '*', action: '*' });
    } else {
        document.querySelectorAll('.perm-check:checked').forEach(cb => {
            permissions.push({ resource: cb.value, action: '*' });
        });
    }

    if (!permissions.length) {
        alert('Select at least one permission');
        return;
    }

    try {
        const resp = await fetch('/api/apikeys', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, permissions, expires_in: expiresIn })
        });
        if (resp.ok) {
            const data = await resp.json();
            // Show the created key (only shown once!)
            document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
            showCreatedAPIKey(data);
        } else {
            alert('Failed to create API key');
        }
    } catch (e) {
        alert('Failed to create API key');
    }
}

function showCreatedAPIKey(data) {
    const keyModal = document.createElement('div');
    keyModal.className = 'modal-overlay';
    keyModal.style.zIndex = '1001';
    keyModal.innerHTML = `
        <div class="modal" style="max-width: 500px;">
            <div class="modal-header">
                <span class="modal-title">API Key Created</span>
            </div>
            <div class="modal-body">
                <p style="color: #ffd400; font-size: 0.85rem; margin-bottom: 1rem;">Copy this key now. You won't be able to see it again!</p>
                <div class="apikey-created">${escapeHtml(data.key)}</div>
                <button class="btn" style="margin-top: 0.5rem;" onclick="navigator.clipboard.writeText('${escapeHtml(data.key)}'); this.textContent='Copied!'">Copy to Clipboard</button>
            </div>
            <div class="modal-footer">
                <button class="btn btn-primary" onclick="this.closest('.modal-overlay').remove(); loadAPIKeysList();">Done</button>
            </div>
        </div>
    `;
    document.body.appendChild(keyModal);
}

async function deleteAPIKey(keyId) {
    if (!confirm('Are you sure you want to delete this API key?')) return;
    try {
        await fetch(`/api/apikeys/${keyId}`, { method: 'DELETE' });
        loadAPIKeysList();
    } catch (e) {
        alert('Failed to delete API key');
    }
}

// =====================================
// Notification Channel Management
// =====================================
const channelTypes = {
    webhook: { name: 'Webhook', icon: '&#128279;', color: '#6b7280' },
    slack: { name: 'Slack', icon: '&#128172;', color: '#4A154B' },
    email: { name: 'Email', icon: '&#128231;', color: '#ea580c' },
    pagerduty: { name: 'PagerDuty', icon: '&#128276;', color: '#06ac38' },
    opsgenie: { name: 'OpsGenie', icon: '&#128680;', color: '#2684ff' },
    msteams: { name: 'MS Teams', icon: '&#128101;', color: '#5558af' },
    discord: { name: 'Discord', icon: '&#127918;', color: '#5865f2' }
};

async function showNotificationChannels() {
    document.getElementById('user-menu-dropdown').classList.remove('show');
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width: 700px;">
            <div class="modal-header">
                <span class="modal-title">Notification Channels</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body" style="padding: 0;">
                <div style="padding: 0.8rem; border-bottom: 1px solid #2f3336; display: flex; justify-content: space-between; align-items: center;">
                    <span style="color: #71767b; font-size: 0.8rem;">Configure where notifications are sent</span>
                    <button class="btn btn-primary" onclick="showCreateChannelModal()">+ Add Channel</button>
                </div>
                <div id="notify-channels-list" style="max-height: 400px; overflow-y: auto;">
                    <div class="empty-state">Loading channels...</div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="showNotificationHistory()">History</button>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
    loadNotifyChannelsList();
}

async function loadNotifyChannelsList() {
    try {
        const resp = await fetch('/api/notify/channels');
        const channels = await resp.json();
        const list = document.getElementById('notify-channels-list');
        if (!channels?.length) {
            list.innerHTML = '<div class="empty-state">No notification channels configured</div>';
            return;
        }
        list.innerHTML = channels.map(ch => {
            const typeInfo = channelTypes[ch.type] || { name: ch.type, icon: '&#128276;', color: '#6b7280' };
            return `
            <div class="apikey-item" style="display: flex; align-items: center; gap: 0.8rem;">
                <div style="width: 36px; height: 36px; background: ${typeInfo.color}; border-radius: 6px; display: flex; align-items: center; justify-content: center; font-size: 1.2rem; flex-shrink: 0;">
                    ${typeInfo.icon}
                </div>
                <div style="flex: 1; min-width: 0;">
                    <div class="apikey-header" style="justify-content: flex-start; gap: 0.5rem;">
                        <span class="apikey-name">${escapeHtml(ch.name)}</span>
                        <span style="font-size: 0.65rem; padding: 0.1rem 0.4rem; background: ${ch.enabled ? '#1a3d2e' : '#4a1919'}; color: ${ch.enabled ? '#00ba7c' : '#f4212e'}; border-radius: 3px;">${ch.enabled ? 'Active' : 'Disabled'}</span>
                    </div>
                    <div class="apikey-meta">
                        <span>${typeInfo.name}</span>
                        ${ch.last_used_at ? `<span>Last used: ${new Date(ch.last_used_at).toLocaleDateString()}</span>` : '<span>Never used</span>'}
                        ${ch.success_rate !== undefined ? `<span>Success: ${ch.success_rate.toFixed(0)}%</span>` : ''}
                    </div>
                </div>
                <div style="display: flex; gap: 0.3rem;">
                    <button class="btn" onclick="testNotifyChannel('${ch.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem;">Test</button>
                    <button class="btn" onclick="editNotifyChannel('${ch.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem;">Edit</button>
                    <button class="btn" onclick="deleteNotifyChannel('${ch.id}')" style="padding: 0.2rem 0.4rem; font-size: 0.65rem; background: #4a1919; color: #f4212e;">Delete</button>
                </div>
            </div>
        `;}).join('');
    } catch (e) {
        document.getElementById('notify-channels-list').innerHTML = '<div class="empty-state">Failed to load channels</div>';
    }
}

function showCreateChannelModal() {
    const createModal = document.createElement('div');
    createModal.className = 'modal-overlay';
    createModal.style.zIndex = '1001';
    createModal.onclick = (e) => { if (e.target === createModal) createModal.remove(); };
    createModal.innerHTML = `
        <div class="modal" style="max-width: 500px;">
            <div class="modal-header">
                <span class="modal-title">Add Notification Channel</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Channel Type</label>
                    <select id="channel-type" class="form-select" onchange="updateChannelConfigForm()">
                        <option value="webhook">Webhook</option>
                        <option value="slack">Slack</option>
                        <option value="email">Email (SMTP)</option>
                        <option value="pagerduty">PagerDuty</option>
                        <option value="opsgenie">OpsGenie</option>
                        <option value="msteams">Microsoft Teams</option>
                        <option value="discord">Discord</option>
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label">Name</label>
                    <input type="text" id="channel-name" class="form-input" placeholder="Production Alerts">
                </div>
                <div id="channel-config-form">
                    <!-- Dynamic config form -->
                </div>
                <div class="form-group">
                    <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                        <input type="checkbox" id="channel-enabled" checked>
                        <span class="form-label" style="margin: 0;">Enabled</span>
                    </label>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createNotifyChannel()">Create Channel</button>
            </div>
        </div>
    `;
    document.body.appendChild(createModal);
    updateChannelConfigForm();
}

function updateChannelConfigForm() {
    const type = document.getElementById('channel-type').value;
    const container = document.getElementById('channel-config-form');

    const forms = {
        webhook: `
            <div class="form-group">
                <label class="form-label">Webhook URL</label>
                <input type="url" id="config-url" class="form-input" placeholder="https://example.com/webhook">
            </div>
            <div class="form-group">
                <label class="form-label">Method</label>
                <select id="config-method" class="form-select">
                    <option value="POST">POST</option>
                    <option value="PUT">PUT</option>
                </select>
            </div>
        `,
        slack: `
            <div class="form-group">
                <label class="form-label">Webhook URL</label>
                <input type="url" id="config-webhook_url" class="form-input" placeholder="https://hooks.slack.com/services/...">
            </div>
            <div class="form-group">
                <label class="form-label">Channel (optional)</label>
                <input type="text" id="config-channel" class="form-input" placeholder="#alerts">
            </div>
        `,
        email: `
            <div class="form-row">
                <div class="form-group">
                    <label class="form-label">SMTP Host</label>
                    <input type="text" id="config-smtp_host" class="form-input" placeholder="smtp.gmail.com">
                </div>
                <div class="form-group">
                    <label class="form-label">Port</label>
                    <input type="number" id="config-smtp_port" class="form-input" placeholder="587" value="587">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label class="form-label">Username</label>
                    <input type="text" id="config-username" class="form-input">
                </div>
                <div class="form-group">
                    <label class="form-label">Password</label>
                    <input type="password" id="config-password" class="form-input">
                </div>
            </div>
            <div class="form-group">
                <label class="form-label">From Address</label>
                <input type="email" id="config-from" class="form-input" placeholder="alerts@example.com">
            </div>
            <div class="form-group">
                <label class="form-label">To Addresses (comma separated)</label>
                <input type="text" id="config-to" class="form-input" placeholder="team@example.com, oncall@example.com">
            </div>
            <div class="form-group">
                <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                    <input type="checkbox" id="config-tls" checked>
                    <span class="form-label" style="margin: 0;">Use TLS</span>
                </label>
            </div>
        `,
        pagerduty: `
            <div class="form-group">
                <label class="form-label">Integration Key (Events API v2)</label>
                <input type="text" id="config-integration_key" class="form-input" placeholder="32 character key">
            </div>
            <div class="form-group">
                <label class="form-label">Default Severity</label>
                <select id="config-default_severity" class="form-select">
                    <option value="">Auto (based on alert severity)</option>
                    <option value="critical">Critical</option>
                    <option value="error">Error</option>
                    <option value="warning">Warning</option>
                    <option value="info">Info</option>
                </select>
            </div>
        `,
        opsgenie: `
            <div class="form-group">
                <label class="form-label">API Key</label>
                <input type="text" id="config-api_key" class="form-input">
            </div>
            <div class="form-group">
                <label class="form-label">Region</label>
                <select id="config-region" class="form-select">
                    <option value="us">US</option>
                    <option value="eu">EU</option>
                </select>
            </div>
            <div class="form-group">
                <label class="form-label">Default Priority</label>
                <select id="config-priority" class="form-select">
                    <option value="">Auto</option>
                    <option value="P1">P1 - Critical</option>
                    <option value="P2">P2 - High</option>
                    <option value="P3">P3 - Medium</option>
                    <option value="P4">P4 - Low</option>
                    <option value="P5">P5 - Informational</option>
                </select>
            </div>
        `,
        msteams: `
            <div class="form-group">
                <label class="form-label">Webhook URL</label>
                <input type="url" id="config-webhook_url" class="form-input" placeholder="https://outlook.office.com/webhook/...">
            </div>
        `,
        discord: `
            <div class="form-group">
                <label class="form-label">Webhook URL</label>
                <input type="url" id="config-webhook_url" class="form-input" placeholder="https://discord.com/api/webhooks/...">
            </div>
            <div class="form-group">
                <label class="form-label">Bot Username (optional)</label>
                <input type="text" id="config-username" class="form-input" placeholder="dogwatch">
            </div>
        `
    };

    container.innerHTML = forms[type] || '';
}

function getChannelConfig() {
    const type = document.getElementById('channel-type').value;
    const config = {};

    const getVal = (id) => document.getElementById(id)?.value || '';
    const getChecked = (id) => document.getElementById(id)?.checked || false;

    switch (type) {
        case 'webhook':
            config.url = getVal('config-url');
            config.method = getVal('config-method');
            break;
        case 'slack':
            config.webhook_url = getVal('config-webhook_url');
            if (getVal('config-channel')) config.channel = getVal('config-channel');
            break;
        case 'email':
            config.smtp_host = getVal('config-smtp_host');
            config.smtp_port = parseInt(getVal('config-smtp_port')) || 587;
            config.username = getVal('config-username');
            config.password = getVal('config-password');
            config.from = getVal('config-from');
            config.to = getVal('config-to').split(',').map(s => s.trim()).filter(s => s);
            config.tls = getChecked('config-tls');
            break;
        case 'pagerduty':
            config.integration_key = getVal('config-integration_key');
            if (getVal('config-default_severity')) config.default_severity = getVal('config-default_severity');
            break;
        case 'opsgenie':
            config.api_key = getVal('config-api_key');
            config.region = getVal('config-region');
            if (getVal('config-priority')) config.priority = getVal('config-priority');
            break;
        case 'msteams':
            config.webhook_url = getVal('config-webhook_url');
            break;
        case 'discord':
            config.webhook_url = getVal('config-webhook_url');
            if (getVal('config-username')) config.username = getVal('config-username');
            break;
    }
    return config;
}

async function createNotifyChannel() {
    const name = document.getElementById('channel-name').value;
    const type = document.getElementById('channel-type').value;
    const enabled = document.getElementById('channel-enabled').checked;
    const config = getChannelConfig();

    if (!name) { alert('Name is required'); return; }

    try {
        const resp = await fetch('/api/notify/channels', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, type, config, enabled })
        });
        if (resp.ok) {
            document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
            loadNotifyChannelsList();
        } else {
            const err = await resp.json();
            alert('Failed to create channel: ' + (err.error || 'Unknown error'));
        }
    } catch (e) {
        alert('Failed to create channel');
    }
}

async function testNotifyChannel(id) {
    try {
        const resp = await fetch(`/api/notify/channels/${id}/test`, { method: 'POST' });
        if (resp.ok) {
            alert('Test notification sent!');
        } else {
            const err = await resp.json();
            alert('Test failed: ' + (err.error || 'Unknown error'));
        }
    } catch (e) {
        alert('Test failed');
    }
}

async function editNotifyChannel(id) {
    try {
        const resp = await fetch(`/api/notify/channels/${id}`);
        const channel = await resp.json();

        const editModal = document.createElement('div');
        editModal.className = 'modal-overlay';
        editModal.style.zIndex = '1001';
        editModal.onclick = (e) => { if (e.target === editModal) editModal.remove(); };

        const typeInfo = channelTypes[channel.type] || { name: channel.type };
        editModal.innerHTML = `
            <div class="modal" style="max-width: 500px;">
                <div class="modal-header">
                    <span class="modal-title">Edit ${typeInfo.name} Channel</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group">
                        <label class="form-label">Name</label>
                        <input type="text" id="edit-channel-name" class="form-input" value="${escapeHtml(channel.name)}">
                    </div>
                    <div class="form-group">
                        <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
                            <input type="checkbox" id="edit-channel-enabled" ${channel.enabled ? 'checked' : ''}>
                            <span class="form-label" style="margin: 0;">Enabled</span>
                        </label>
                    </div>
                    <p style="color: #71767b; font-size: 0.75rem; margin-top: 1rem;">To change channel configuration, delete and recreate the channel.</p>
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                    <button class="btn btn-primary" onclick="saveNotifyChannel('${id}', '${channel.type}')">Save</button>
                </div>
            </div>
        `;
        document.body.appendChild(editModal);
    } catch (e) {
        alert('Failed to load channel');
    }
}

async function saveNotifyChannel(id, type) {
    const name = document.getElementById('edit-channel-name').value;
    const enabled = document.getElementById('edit-channel-enabled').checked;

    try {
        const resp = await fetch(`/api/notify/channels/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name, type, enabled, config: {} })
        });
        if (resp.ok) {
            document.querySelector('.modal-overlay[style*="z-index: 1001"]')?.remove();
            loadNotifyChannelsList();
        } else {
            alert('Failed to save channel');
        }
    } catch (e) {
        alert('Failed to save channel');
    }
}

async function deleteNotifyChannel(id) {
    if (!confirm('Are you sure you want to delete this notification channel?')) return;
    try {
        await fetch(`/api/notify/channels/${id}`, { method: 'DELETE' });
        loadNotifyChannelsList();
    } catch (e) {
        alert('Failed to delete channel');
    }
}

async function showNotificationHistory() {
    try {
        const resp = await fetch('/api/notify/history');
        const logs = await resp.json();

        const historyModal = document.createElement('div');
        historyModal.className = 'modal-overlay';
        historyModal.style.zIndex = '1001';
        historyModal.onclick = (e) => { if (e.target === historyModal) historyModal.remove(); };
        historyModal.innerHTML = `
            <div class="modal" style="max-width: 700px;">
                <div class="modal-header">
                    <span class="modal-title">Notification History</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body" style="max-height: 400px; overflow-y: auto; padding: 0;">
                    ${!logs?.length ? '<div class="empty-state">No notifications sent yet</div>' : logs.map(log => `
                        <div style="padding: 0.6rem 1rem; border-bottom: 1px solid #2f3336; display: flex; align-items: center; gap: 0.8rem;">
                            <div style="width: 10px; height: 10px; border-radius: 50%; background: ${log.status === 'sent' ? '#00ba7c' : '#f4212e'}; flex-shrink: 0;"></div>
                            <div style="flex: 1; min-width: 0;">
                                <div style="font-weight: 500; font-size: 0.85rem;">${escapeHtml(log.notification?.title || 'Notification')}</div>
                                <div style="font-size: 0.7rem; color: #71767b;">
                                    ${escapeHtml(log.channel_name)} (${log.channel_type}) - ${new Date(log.sent_at).toLocaleString()}
                                    ${log.response_time ? ` - ${log.response_time}ms` : ''}
                                </div>
                                ${log.error ? `<div style="font-size: 0.7rem; color: #f4212e; margin-top: 0.2rem;">${escapeHtml(log.error)}</div>` : ''}
                            </div>
                        </div>
                    `).join('')}
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                </div>
            </div>
        `;
        document.body.appendChild(historyModal);
    } catch (e) {
        alert('Failed to load notification history');
    }
}

// Check auth on page load
checkAuth();

// =====================================
// Widget definitions
// =====================================
const defaultLayout = [
    // =============================================
    // HERO SECTION: System health at a glance
    // =============================================
    // Row 0-1: System metrics (4 equal cards)
    { id: 'cpu', x: 0, y: 0, w: 3, h: 2, minW: 2, minH: 2 },
    { id: 'mem', x: 3, y: 0, w: 3, h: 2, minW: 2, minH: 2 },
    { id: 'disk', x: 6, y: 0, w: 3, h: 2, minW: 2, minH: 2 },
    { id: 'net', x: 9, y: 0, w: 3, h: 2, minW: 2, minH: 2 },

    // =============================================
    // PRIMARY SECTION: Service topology + Alerts
    // =============================================
    // Row 2-6: Service map (left 8) + Alerts (right 4) - critical ops view
    { id: 'svcmap', x: 0, y: 2, w: 8, h: 5, minW: 6, minH: 4 },
    { id: 'alerting', x: 8, y: 2, w: 4, h: 5, minW: 3, minH: 4 },

    // =============================================
    // SECONDARY SECTION: SLOs + Request flow
    // =============================================
    // Row 7-10: SLOs (left 4) + Traces (center 4) + Endpoints (right 4)
    { id: 'slos', x: 0, y: 7, w: 4, h: 4, minW: 3, minH: 3 },
    { id: 'traces', x: 4, y: 7, w: 4, h: 4, minW: 3, minH: 3 },
    { id: 'endpoints', x: 8, y: 7, w: 4, h: 4, minW: 3, minH: 3 },

    // =============================================
    // CHARTS SECTION: Time-series trends
    // =============================================
    // Row 11-13: CPU + Memory charts side by side
    { id: 'cpuchart', x: 0, y: 11, w: 6, h: 3, minW: 4, minH: 3 },
    { id: 'memchart', x: 6, y: 11, w: 6, h: 3, minW: 4, minH: 3 },

    // =============================================
    // OPERATIONAL SECTION: Logs + Recent activity
    // =============================================
    // Row 14-17: Logs (left 6) + Deployments/Incidents stacked (right 6)
    { id: 'logs', x: 0, y: 14, w: 6, h: 4, minW: 5, minH: 3 },
    { id: 'deployments', x: 6, y: 14, w: 6, h: 4, minW: 4, minH: 3 },

    // Row 18-21: Watches/Incidents + Synthetics
    { id: 'incidents', x: 0, y: 18, w: 6, h: 4, minW: 4, minH: 3 },
    { id: 'synthetics', x: 6, y: 18, w: 6, h: 4, minW: 4, minH: 3 },

    // =============================================
    // INFRASTRUCTURE SECTION: Containers + K8s
    // =============================================
    // Row 22-25: Containers + Cluster side by side
    { id: 'containers', x: 0, y: 22, w: 6, h: 4, minW: 4, minH: 3 },
    { id: 'cluster', x: 6, y: 22, w: 6, h: 4, minW: 4, minH: 3 },

    // Row 26-30: Kubernetes (full width - complex widget)
    { id: 'kubernetes', x: 0, y: 26, w: 12, h: 5, minW: 8, minH: 4 },

    // =============================================
    // ANALYTICS SECTION: Patterns + Anomalies
    // =============================================
    // Row 31-34: Log patterns + Anomaly detection
    { id: 'patterns', x: 0, y: 31, w: 6, h: 4, minW: 4, minH: 3 },
    { id: 'anomaly', x: 6, y: 31, w: 6, h: 4, minW: 4, minH: 3 },

    // =============================================
    // CATALOG SECTION: Service registry + Status
    // =============================================
    // Row 35-39: Service catalog (left 7) + Status page (right 5)
    { id: 'catalog', x: 0, y: 35, w: 7, h: 5, minW: 5, minH: 4 },
    { id: 'statuspage', x: 7, y: 35, w: 5, h: 5, minW: 4, minH: 4 },

    // =============================================
    // DEEP DIVE SECTION: Profiling + Correlation
    // =============================================
    // Row 40-43: Flamegraph + Correlation
    { id: 'flamegraph', x: 0, y: 40, w: 6, h: 4, minW: 4, minH: 3 },
    { id: 'correlation', x: 6, y: 40, w: 6, h: 4, minW: 4, minH: 3 },

    // =============================================
    // DATA SECTION: DB + Cardinality + Stats
    // =============================================
    // Row 44-48: DB Watch (left 8) + Stats (right 4)
    { id: 'dbwatch', x: 0, y: 44, w: 8, h: 5, minW: 6, minH: 4 },
    { id: 'cardinality', x: 8, y: 44, w: 4, h: 5, minW: 3, minH: 4 },

    // =============================================
    // COST SECTION: Value proposition (marketing)
    // =============================================
    // Row 49-52: Cost Intelligence (full width - key differentiator)
    { id: 'costintel', x: 0, y: 49, w: 12, h: 4, minW: 8, minH: 3 },

    // =============================================
    // DETAIL SECTION: Processes + Connections
    // =============================================
    // Row 53-56: Lower-priority detailed views
    { id: 'procs', x: 0, y: 53, w: 6, h: 4, minW: 3, minH: 3 },
    { id: 'connlist', x: 6, y: 53, w: 6, h: 4, minW: 3, minH: 3 },

    // Quick stats (small cards - can be placed in gaps or at bottom)
    { id: 'conns', x: 0, y: 57, w: 2, h: 2, minW: 2, minH: 2 },
    { id: 'reqs', x: 2, y: 57, w: 2, h: 2, minW: 2, minH: 2 },
    { id: 'errs', x: 4, y: 57, w: 2, h: 2, minW: 2, minH: 2 },
    { id: 'watches', x: 6, y: 57, w: 6, h: 2, minW: 4, minH: 2 },
];

function widgetCPU() {
    return `<div class="widget-header"><span class="widget-title">CPU</span></div>
        <div class="widget-body">
            <div class="metric-big" id="cpu-percent">--%</div>
            <div class="metric-bar"><div class="metric-bar-fill cpu" id="cpu-bar" style="width:0%"></div></div>
            <div class="metric-detail"><span>Load: <span id="load-avg">--</span></span><span>IO: <span id="cpu-iowait">--%</span></span></div>
        </div>`;
}
function widgetMem() {
    return `<div class="widget-header"><span class="widget-title">Memory</span></div>
        <div class="widget-body">
            <div class="metric-big" id="mem-percent">--%</div>
            <div class="metric-bar"><div class="metric-bar-fill mem" id="mem-bar" style="width:0%"></div></div>
            <div class="metric-detail"><span id="mem-used">--</span><span id="mem-total">--</span></div>
        </div>`;
}
function widgetDisk() {
    return `<div class="widget-header"><span class="widget-title">Disk I/O</span></div>
        <div class="widget-body">
            <div class="metric-big" id="disk-io">--</div>
            <div class="metric-bar"><div class="metric-bar-fill disk" id="disk-bar" style="width:0%"></div></div>
            <div class="metric-detail"><span>R: <span id="disk-read">--</span></span><span>W: <span id="disk-write">--</span></span></div>
        </div>`;
}
function widgetNet() {
    return `<div class="widget-header"><span class="widget-title">Network</span></div>
        <div class="widget-body">
            <div class="metric-big" id="net-io">--</div>
            <div class="metric-bar"><div class="metric-bar-fill net" id="net-bar" style="width:0%"></div></div>
            <div class="metric-detail"><span>RX: <span id="net-rx">--</span></span><span>TX: <span id="net-tx">--</span></span></div>
        </div>`;
}
function widgetStat(title, id, isError) {
    return `<div class="widget-header"><span class="widget-title">${title}</span></div>
        <div class="widget-body" style="display:flex;align-items:center;justify-content:center;">
            <div class="stat-big ${isError ? 'error' : ''}" id="${id}">-</div>
        </div>`;
}
function widgetServiceMap() {
    return `<div class="widget-header">
            <span class="widget-title">Service Map</span>
            <div style="display:flex;gap:0.3rem;align-items:center;">
                <select id="svcmap-filter" class="trace-select" style="height:26px;font-size:0.7rem;" onchange="filterServiceMap()">
                    <option value="">All Nodes</option>
                    <option value="services">Services</option>
                    <option value="external">External</option>
                    <option value="processes">Processes</option>
                </select>
                <button class="btn" onclick="toggleSvcMapLayout()" title="Toggle Layout">⟲</button>
                <button class="btn" onclick="updateServiceMap()" title="Refresh">↻</button>
            </div>
        </div>
        <div class="widget-body no-pad svcmap-container" id="svcmap-container">
            <div class="svcmap-stats" id="svcmap-stats"></div>
            <svg id="service-map"></svg>
            <div class="svcmap-zoom-controls">
                <button onclick="svcMapZoom(1.2)" title="Zoom In">+</button>
                <button onclick="svcMapZoom(0.8)" title="Zoom Out">−</button>
                <button onclick="svcMapReset()" title="Reset View">⌂</button>
            </div>
            <div class="svcmap-legend">
                <div class="svcmap-legend-item"><div class="svcmap-legend-dot" style="background:#00ba7c"></div>Healthy</div>
                <div class="svcmap-legend-item"><div class="svcmap-legend-dot" style="background:#ffd400"></div>Degraded</div>
                <div class="svcmap-legend-item"><div class="svcmap-legend-dot" style="background:#f4212e"></div>Error</div>
                <div class="svcmap-legend-item"><div class="svcmap-legend-dot" style="background:#536471"></div>External</div>
            </div>
            <div class="svcmap-tooltip" id="svcmap-tooltip"></div>
            <div class="svcmap-detail" id="svcmap-detail">
                <div class="svcmap-detail-header">
                    <span class="svcmap-detail-title" id="svcmap-detail-title">Node Details</span>
                    <button class="svcmap-detail-close" onclick="closeSvcMapDetail()">×</button>
                </div>
                <div class="svcmap-detail-body" id="svcmap-detail-body"></div>
                <div class="svcmap-detail-actions">
                    <button class="btn" onclick="viewServiceLogs()">View Logs</button>
                    <button class="btn" onclick="viewServiceTraces()">View Traces</button>
                </div>
            </div>
        </div>`;
}
function widgetChart(title, id) {
    return `<div class="widget-header">
            <span class="widget-title">${title}</span>
            <div class="time-btns">
                <button class="time-btn active" data-dur="15m">15m</button>
                <button class="time-btn" data-dur="1h">1h</button>
                <button class="time-btn" data-dur="6h">6h</button>
            </div>
        </div>
        <div class="widget-body" style="position:relative;"><div class="chart-wrap"><canvas id="${id}"></canvas></div></div>`;
}
function widgetEndpoints() {
    return `<div class="widget-header"><span class="widget-title">HTTP Endpoints</span></div>
        <div class="widget-body no-pad" style="overflow:auto;">
            <table><thead><tr><th>Method</th><th>Path</th><th>Count</th><th>Err%</th><th>P50</th><th>P99</th></tr></thead>
            <tbody id="endpoints-body"><tr><td colspan="6" class="empty-state">No data</td></tr></tbody></table>
        </div>`;
}
function widgetConnections() {
    return `<div class="widget-header"><span class="widget-title">Connections</span></div>
        <div class="widget-body no-pad" style="overflow:auto;">
            <table><thead><tr><th>Process</th><th>Remote</th><th>Count</th></tr></thead>
            <tbody id="connections-body"><tr><td colspan="3" class="empty-state">No data</td></tr></tbody></table>
        </div>`;
}
function widgetProcesses() {
    return `<div class="widget-header"><span class="widget-title">Top Processes</span></div>
        <div class="widget-body no-pad" style="overflow:auto;">
            <table><thead><tr><th>PID</th><th>Name</th><th>CPU%</th><th>Mem</th></tr></thead>
            <tbody id="processes-body"><tr><td colspan="4" class="empty-state">Loading...</td></tr></tbody></table>
        </div>`;
}
function widgetFlameGraph() {
    return `<div class="widget-header">
            <span class="widget-title">CPU Flame Graph</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="clearFlameGraph()">Clear</button>
                <button class="btn" onclick="updateFlameGraph()">Refresh</button>
            </div>
        </div>
        <div class="widget-body no-pad flamegraph-container">
            <div class="flamegraph-toolbar">
                <input type="text" class="flamegraph-search" id="flamegraph-search" placeholder="Search functions..." oninput="searchFlameGraph(this.value)">
                <div class="flamegraph-divider"></div>
                <div class="flamegraph-btn-group">
                    <button class="flamegraph-btn active" id="flamegraph-flame-btn" onclick="setFlameMode('flame')" title="Flame Graph">▲</button>
                    <button class="flamegraph-btn" id="flamegraph-icicle-btn" onclick="setFlameMode('icicle')" title="Icicle Graph">▼</button>
                </div>
                <div class="flamegraph-divider"></div>
                <button class="flamegraph-btn" id="flamegraph-diff-btn" onclick="toggleDiffMode()" title="Compare Profiles">Diff</button>
                <div class="flamegraph-stats" id="flamegraph-stats"></div>
            </div>
            <div class="flamegraph-breadcrumbs" id="flamegraph-breadcrumbs" style="display:none;"></div>
            <div class="flamegraph-view" id="flamegraph-view">
                <div class="flamegraph-canvas" id="flamegraph"></div>
            </div>
            <div class="flamegraph-legend" id="flamegraph-legend">
                <div class="flamegraph-legend-item"><div class="flamegraph-legend-color" style="background:linear-gradient(135deg,#f97316,#ea580c)"></div>Hot (>10%)</div>
                <div class="flamegraph-legend-item"><div class="flamegraph-legend-color" style="background:linear-gradient(135deg,#fbbf24,#f59e0b)"></div>Warm (5-10%)</div>
                <div class="flamegraph-legend-item"><div class="flamegraph-legend-color" style="background:linear-gradient(135deg,#3b82f6,#2563eb)"></div>Normal</div>
                <div class="flamegraph-legend-item"><div class="flamegraph-legend-color" style="background:linear-gradient(135deg,#ef4444,#dc2626)"></div>Kernel/Unknown</div>
            </div>
            <div class="flamegraph-tooltip" id="flamegraph-tooltip"></div>
        </div>`;
}
function widgetTraces() {
    return `<div class="widget-header">
            <span class="widget-title">Distributed Traces</span>
            <div class="trace-controls">
                <select id="trace-service-filter" class="trace-select" onchange="loadTraces()">
                    <option value="">All Services</option>
                </select>
                <button class="btn" onclick="loadTraces()">Refresh</button>
            </div>
        </div>
        <div class="widget-body no-pad" style="display:flex;flex-direction:column;height:100%;">
            <div class="trace-list" id="trace-list">
                <div class="empty-state">No traces yet. Configure OTLP export to http://localhost:9999/v1/traces</div>
            </div>
            <div id="trace-detail" style="flex:1;overflow:auto;">
                <div class="no-trace-selected">Select a trace to view details</div>
            </div>
        </div>`;
}
function widgetWatches() {
    return `<div class="widget-header">
            <span class="widget-title">Watches</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showChannels()">Channels</button>
                <button class="btn btn-primary" onclick="showCreateWatch()">+ New</button>
            </div>
        </div>
        <div class="widget-body no-pad" id="watches-list" style="overflow:auto;">
            <div class="empty-state">No watches configured</div>
        </div>`;
}
function widgetLogs() {
    return `<div class="widget-header">
            <span class="widget-title">Logs</span>
            <div style="display:flex;gap:0.3rem;align-items:center;">
                <button class="btn" id="log-tail-btn" onclick="toggleLogTail()" title="Live tail">
                    <span id="log-tail-icon">▶</span> Live
                </button>
                <button class="btn" onclick="refreshLogs()">Refresh</button>
            </div>
        </div>
        <div class="log-controls">
            <div style="display:flex;gap:0.3rem;flex:1;min-width:200px;">
                <input type="text" id="log-search" placeholder="Search logs..." onkeyup="if(event.key==='Enter')searchLogs()" style="flex:1;">
                <button class="btn btn-primary" onclick="searchLogs()" style="padding:0.3rem 0.6rem;">Search</button>
            </div>
            <select id="log-level" onchange="updateLogFilters()">
                <option value="">All Levels</option>
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warn</option>
                <option value="error">Error</option>
                <option value="fatal">Fatal</option>
            </select>
            <select id="log-service" onchange="updateLogFilters()">
                <option value="">All Services</option>
            </select>
            <select id="log-time" onchange="updateLogFilters()">
                <option value="15m">Last 15m</option>
                <option value="1h" selected>Last 1h</option>
                <option value="6h">Last 6h</option>
                <option value="24h">Last 24h</option>
                <option value="7d">Last 7d</option>
                <option value="custom">Custom range...</option>
            </select>
        </div>
        <div id="log-filter-pills" style="display:none;padding:0.3rem 0.8rem;background:#1a1d21;border-bottom:1px solid #2f3336;"></div>
        <div class="widget-body no-pad" id="logs-list" style="overflow:auto;">
            <div class="empty-state">No logs yet</div>
        </div>`;
}
function widgetSynthetics() {
    return `<div class="widget-header">
            <span class="widget-title">Synthetic Checks</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn btn-primary" onclick="showCreateSynthetic()">+ New</button>
            </div>
        </div>
        <div class="widget-body no-pad" id="synthetics-list" style="overflow:auto;">
            <div class="empty-state">No synthetic checks configured</div>
        </div>`;
}
function widgetSLOs() {
    return `<div class="widget-header">
            <span class="widget-title">Service Level Objectives</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn btn-primary" onclick="showCreateSLO()">+ New</button>
            </div>
        </div>
        <div class="widget-body no-pad" id="slos-list" style="overflow:auto;">
            <div class="empty-state">No SLOs configured</div>
        </div>`;
}
function widgetPatterns() {
    return `<div class="widget-header">
            <span class="widget-title">Log Patterns</span>
            <div style="display:flex;gap:0.3rem;">
                <select id="pattern-filter" onchange="loadPatterns()">
                    <option value="all">All Patterns</option>
                    <option value="new">New (24h)</option>
                    <option value="increasing">Increasing</option>
                </select>
                <button class="btn" onclick="loadPatterns()">Refresh</button>
            </div>
        </div>
        <div class="pattern-stats" id="pattern-stats"></div>
        <div class="widget-body no-pad" id="patterns-list" style="overflow:auto;">
            <div class="empty-state">No patterns detected yet</div>
        </div>`;
}
function widgetContainers() {
    return `<div class="widget-header">
            <span class="widget-title">Containers</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="loadContainers()">Refresh</button>
            </div>
        </div>
        <div class="container-summary" id="container-summary"></div>
        <div class="widget-body no-pad" id="containers-list" style="overflow:auto;">
            <div class="empty-state">No containers found</div>
        </div>`;
}
function widgetDeployments() {
    return `<div class="widget-header">
            <span class="widget-title">Deployments</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showNewDeployModal()">+ Deploy</button>
                <button class="btn" onclick="loadDeployments()">Refresh</button>
            </div>
        </div>
        <div class="deploy-summary" id="deploy-summary"></div>
        <div class="widget-body no-pad" id="deployments-list" style="overflow:auto;">
            <div class="empty-state">No deployments recorded</div>
        </div>`;
}

function widgetIncidents() {
    return `<div class="widget-header">
            <span class="widget-title">Incidents</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showNewIncidentModal()">+ Incident</button>
                <button class="btn" onclick="loadIncidents()">Refresh</button>
            </div>
        </div>
        <div class="incident-summary" id="incident-summary">
            <div class="incident-stat">
                <span class="incident-stat-value" id="inc-active">0</span>
                <span class="incident-stat-label">Active</span>
            </div>
            <div class="incident-stat">
                <span class="incident-stat-value" id="inc-triggered" style="color:#f4212e">0</span>
                <span class="incident-stat-label">Triggered</span>
            </div>
            <div class="incident-stat">
                <span class="incident-stat-value" id="inc-acked" style="color:#ffd400">0</span>
                <span class="incident-stat-label">Acked</span>
            </div>
            <div id="oncall-display" style="margin-left:auto;"></div>
        </div>
        <div class="widget-body no-pad" id="incidents-list" style="overflow:auto;">
            <div class="empty-state">No active incidents</div>
        </div>`;
}

function widgetCluster() {
    return `<div class="widget-header">
            <span class="widget-title">Federation Cluster</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showJoinClusterModal()">Join Node</button>
                <button class="btn" onclick="loadCluster()">Refresh</button>
            </div>
        </div>
        <div class="cluster-summary" id="cluster-summary">
            <div class="cluster-stat">
                <span class="cluster-stat-value" id="cluster-enabled">-</span>
                <span class="cluster-stat-label">Status</span>
            </div>
            <div class="cluster-stat">
                <span class="cluster-stat-value" id="cluster-nodes">0</span>
                <span class="cluster-stat-label">Nodes</span>
            </div>
            <div class="cluster-stat">
                <span class="cluster-stat-value" id="cluster-incidents">0</span>
                <span class="cluster-stat-label">Active Incidents</span>
            </div>
            <div id="cluster-local" style="margin-left:auto;font-size:0.75rem;color:#888;"></div>
        </div>
        <div class="widget-body no-pad" id="cluster-nodes-list" style="overflow:auto;">
            <div class="empty-state">Federation not enabled. Start with --cluster flag.</div>
        </div>`;
}

function widgetStatusPage() {
    return `<div class="widget-header">
            <span class="widget-title">Status Pages</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showCreateStatusPage()">+ New Page</button>
                <button class="btn" onclick="showCreateComponent()">+ Component</button>
                <button class="btn" onclick="loadStatusPages()">Refresh</button>
            </div>
        </div>
        <div class="statuspage-summary" id="statuspage-summary">
            <div class="statuspage-overall operational" id="statuspage-overall">All Systems Operational</div>
            <div class="statuspage-stat">
                <span class="statuspage-stat-value" id="sp-pages">0</span>
                <span class="statuspage-stat-label">Pages</span>
            </div>
            <div class="statuspage-stat">
                <span class="statuspage-stat-value" id="sp-components">0</span>
                <span class="statuspage-stat-label">Components</span>
            </div>
            <div class="statuspage-stat">
                <span class="statuspage-stat-value" id="sp-incidents">0</span>
                <span class="statuspage-stat-label">Active Incidents</span>
            </div>
        </div>
        <div class="widget-body no-pad" id="statuspage-list" style="overflow:auto;">
            <div class="empty-state">No status pages configured. Click "+ New Page" to create one.</div>
        </div>`;
}

function widgetCatalog() {
    return `<div class="widget-header">
            <span class="widget-title">Service Catalog</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showCreateService()">+ Service</button>
                <button class="btn" onclick="showCreateTeam()">+ Team</button>
                <button class="btn" onclick="loadCatalog()">Refresh</button>
            </div>
        </div>
        <div class="catalog-summary" id="catalog-summary">
            <div class="catalog-stat">
                <span class="catalog-stat-value" id="cat-total">0</span>
                <span class="catalog-stat-label">Services</span>
            </div>
            <div class="catalog-stat">
                <span class="catalog-stat-value" id="cat-critical" style="color:#f4212e">0</span>
                <span class="catalog-stat-label">Critical</span>
            </div>
            <div class="catalog-stat">
                <span class="catalog-stat-value" id="cat-healthy" style="color:#00ba7c">0</span>
                <span class="catalog-stat-label">Healthy</span>
            </div>
            <div class="catalog-stat">
                <span class="catalog-stat-value" id="cat-unhealthy" style="color:#f4212e">0</span>
                <span class="catalog-stat-label">Unhealthy</span>
            </div>
            <div class="catalog-filters">
                <select id="cat-tier-filter" onchange="loadCatalog()">
                    <option value="">All Tiers</option>
                    <option value="critical">Critical</option>
                    <option value="high">High</option>
                    <option value="medium">Medium</option>
                    <option value="low">Low</option>
                </select>
                <select id="cat-health-filter" onchange="loadCatalog()">
                    <option value="">All Health</option>
                    <option value="healthy">Healthy</option>
                    <option value="degraded">Degraded</option>
                    <option value="unhealthy">Unhealthy</option>
                </select>
            </div>
        </div>
        <div class="widget-body no-pad" id="catalog-list" style="overflow:auto;">
            <div class="empty-state">No services in catalog. Click "+ Service" to add one.</div>
        </div>`;
}

function widgetCorrelation() {
    return `<div class="widget-header">
            <span class="widget-title">Correlation Engine</span>
            <div style="display:flex;gap:0.3rem;">
                <select class="trace-select" id="corr-service-select" onchange="loadServiceTimeline()">
                    <option value="">Select Service</option>
                </select>
                <select class="trace-select" id="corr-timerange" onchange="loadServiceTimeline()">
                    <option value="1h">Last 1h</option>
                    <option value="6h">Last 6h</option>
                    <option value="24h">Last 24h</option>
                </select>
                <button class="btn" onclick="loadCorrelations()">Refresh</button>
            </div>
        </div>
        <div class="correlation-summary" id="corr-summary" style="display:flex;gap:1rem;padding:0.5rem;background:#1a1a1a;border-bottom:1px solid #333;">
            <div class="corr-stat" style="text-align:center;">
                <span id="corr-deploy-incidents" style="font-size:1.5rem;font-weight:bold;color:#f4212e;">0</span>
                <div style="font-size:0.75rem;color:#888;">Deploy→Incident</div>
            </div>
            <div class="corr-stat" style="text-align:center;">
                <span id="corr-total-events" style="font-size:1.5rem;font-weight:bold;color:#1d9bf0;">0</span>
                <div style="font-size:0.75rem;color:#888;">Timeline Events</div>
            </div>
            <div class="corr-stat" style="text-align:center;">
                <span id="corr-error-traces" style="font-size:1.5rem;font-weight:bold;color:#ff6b35;">0</span>
                <div style="font-size:0.75rem;color:#888;">Error Traces</div>
            </div>
        </div>
        <div style="display:flex;height:calc(100% - 90px);">
            <div id="corr-correlations" style="width:40%;border-right:1px solid #333;overflow:auto;padding:0.5rem;">
                <div style="font-weight:bold;margin-bottom:0.5rem;color:#888;">Deploy → Incident Correlations</div>
                <div id="corr-deploy-list">
                    <div class="empty-state" style="padding:1rem;text-align:center;color:#666;">No correlations detected</div>
                </div>
            </div>
            <div id="corr-timeline" style="width:60%;overflow:auto;padding:0.5rem;">
                <div style="font-weight:bold;margin-bottom:0.5rem;color:#888;">Service Timeline</div>
                <div id="corr-timeline-list">
                    <div class="empty-state" style="padding:1rem;text-align:center;color:#666;">Select a service to view timeline</div>
                </div>
            </div>
        </div>`;
}

function widgetKubernetes() {
    return `<div class="widget-header">
            <span class="widget-title">Kubernetes</span>
            <div style="display:flex;gap:0.3rem;">
                <select class="trace-select" id="k8s-namespace-select" onchange="loadKubernetes()">
                    <option value="">All Namespaces</option>
                </select>
                <button class="btn" onclick="loadKubernetes()">Refresh</button>
            </div>
        </div>
        <div class="k8s-summary" id="k8s-summary">
            <div class="k8s-stat">
                <span class="k8s-stat-value" id="k8s-pods">-</span>
                <span class="k8s-stat-label">Pods</span>
            </div>
            <div class="k8s-stat">
                <span class="k8s-stat-value" id="k8s-deployments">-</span>
                <span class="k8s-stat-label">Deployments</span>
            </div>
            <div class="k8s-stat">
                <span class="k8s-stat-value" id="k8s-services">-</span>
                <span class="k8s-stat-label">Services</span>
            </div>
            <div class="k8s-stat">
                <span class="k8s-stat-value" id="k8s-nodes">-</span>
                <span class="k8s-stat-label">Nodes</span>
            </div>
            <div id="k8s-cluster-name" style="margin-left:auto;font-size:0.75rem;color:#888;"></div>
        </div>
        <div class="k8s-tabs" id="k8s-tabs">
            <button class="k8s-tab active" data-tab="pods" onclick="switchK8sTab('pods')">Pods</button>
            <button class="k8s-tab" data-tab="deploys" onclick="switchK8sTab('deploys')">Deployments</button>
            <button class="k8s-tab" data-tab="services" onclick="switchK8sTab('services')">Services</button>
            <button class="k8s-tab" data-tab="nodes" onclick="switchK8sTab('nodes')">Nodes</button>
            <button class="k8s-tab" data-tab="events" onclick="switchK8sTab('events')">Events</button>
        </div>
        <div class="widget-body no-pad" style="display:flex;flex-direction:column;flex:1;">
            <div class="k8s-content active" id="k8s-pods-content">
                <div class="empty-state">No pods found</div>
            </div>
            <div class="k8s-content" id="k8s-deploys-content">
                <div class="empty-state">No deployments found</div>
            </div>
            <div class="k8s-content" id="k8s-services-content">
                <div class="empty-state">No services found</div>
            </div>
            <div class="k8s-content" id="k8s-nodes-content">
                <div class="empty-state">No nodes found</div>
            </div>
            <div class="k8s-content" id="k8s-events-content">
                <div class="empty-state">No events found</div>
            </div>
        </div>`;
}

function widgetAnomaly() {
    return `<div class="widget-header">
            <span class="widget-title">Anomaly Detection</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="loadAnomalies()">Refresh</button>
            </div>
        </div>
        <div class="anomaly-summary" id="anomaly-summary">
            <div class="anomaly-stat">
                <span class="anomaly-stat-value critical" id="anomaly-critical">0</span>
                <span class="anomaly-stat-label">Critical</span>
            </div>
            <div class="anomaly-stat">
                <span class="anomaly-stat-value warning" id="anomaly-warning">0</span>
                <span class="anomaly-stat-label">Warning</span>
            </div>
            <div class="anomaly-stat">
                <span class="anomaly-stat-value" id="anomaly-total">0</span>
                <span class="anomaly-stat-label">Total (24h)</span>
            </div>
            <div class="anomaly-stat">
                <span class="anomaly-stat-value" id="anomaly-metrics">0</span>
                <span class="anomaly-stat-label">Metrics</span>
            </div>
        </div>
        <div class="widget-body no-pad" id="anomaly-list" style="overflow:auto;">
            <div class="empty-state">No anomalies detected</div>
        </div>`;
}

function widgetAlerting() {
    return `<div class="widget-header">
            <span class="widget-title">Alerts</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showNewAlertRuleModal()">+ Rule</button>
                <button class="btn" onclick="showNewSilenceModal()">Silence</button>
                <button class="btn" onclick="loadAlerts()">Refresh</button>
            </div>
        </div>
        <div class="alerting-summary" id="alerting-summary">
            <div class="alerting-stat">
                <span class="alerting-stat-value firing" id="alerts-firing">0</span>
                <span class="alerting-stat-label">Firing</span>
            </div>
            <div class="alerting-stat">
                <span class="alerting-stat-value pending" id="alerts-pending">0</span>
                <span class="alerting-stat-label">Pending</span>
            </div>
            <div class="alerting-stat">
                <span class="alerting-stat-value silenced" id="alerts-silenced">0</span>
                <span class="alerting-stat-label">Silences</span>
            </div>
            <div class="alerting-stat">
                <span class="alerting-stat-value" id="alerts-rules">0</span>
                <span class="alerting-stat-label">Rules</span>
            </div>
        </div>
        <div class="alerting-tabs" id="alerting-tabs">
            <button class="alerting-tab active" data-tab="alerts" onclick="switchAlertingTab('alerts')">Alerts</button>
            <button class="alerting-tab" data-tab="rules" onclick="switchAlertingTab('rules')">Rules</button>
            <button class="alerting-tab" data-tab="silences" onclick="switchAlertingTab('silences')">Silences</button>
        </div>
        <div class="widget-body no-pad" style="display:flex;flex-direction:column;flex:1;">
            <div class="alerting-content active" id="alerting-alerts-content">
                <div class="empty-state">No firing alerts</div>
            </div>
            <div class="alerting-content" id="alerting-rules-content">
                <div class="empty-state">No alert rules configured</div>
            </div>
            <div class="alerting-content" id="alerting-silences-content">
                <div class="empty-state">No active silences</div>
            </div>
        </div>`;
}

function widgetNotifications() {
    return `<div class="widget-header">
            <span class="widget-title">Notifications</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showNotificationChannels()">Manage</button>
                <button class="btn" onclick="loadNotifyWidget()">Refresh</button>
            </div>
        </div>
        <div class="notify-summary" id="notify-summary">
            <div class="notify-stat">
                <span class="notify-stat-value" id="notify-channels">0</span>
                <span class="notify-stat-label">Channels</span>
            </div>
            <div class="notify-stat">
                <span class="notify-stat-value" id="notify-enabled">0</span>
                <span class="notify-stat-label">Enabled</span>
            </div>
            <div class="notify-stat">
                <span class="notify-stat-value success" id="notify-sent">0</span>
                <span class="notify-stat-label">Sent (24h)</span>
            </div>
            <div class="notify-stat">
                <span class="notify-stat-value failed" id="notify-failed">0</span>
                <span class="notify-stat-label">Failed</span>
            </div>
        </div>
        <div class="notify-tabs" id="notify-tabs">
            <button class="notify-tab active" data-tab="channels" onclick="switchNotifyTab('channels')">Channels</button>
            <button class="notify-tab" data-tab="history" onclick="switchNotifyTab('history')">History</button>
        </div>
        <div class="widget-body no-pad" style="display:flex;flex-direction:column;flex:1;">
            <div class="notify-content active" id="notify-channels-content">
                <div class="empty-state">No notification channels configured</div>
            </div>
            <div class="notify-content" id="notify-history-content">
                <div class="empty-state">No recent notifications</div>
            </div>
        </div>`;
}

function widgetOnCall() {
    return `<div class="widget-header">
            <span class="widget-title">On-Call</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="showNewScheduleModal()">+ Schedule</button>
                <button class="btn" onclick="loadOnCallWidget()">Refresh</button>
            </div>
        </div>
        <div class="oncall-summary" id="oncall-summary">
            <div class="oncall-stat">
                <span class="oncall-stat-value" id="oncall-schedules-count">0</span>
                <span class="oncall-stat-label">Schedules</span>
            </div>
            <div class="oncall-stat">
                <span class="oncall-stat-value" id="oncall-policies-count">0</span>
                <span class="oncall-stat-label">Policies</span>
            </div>
            <div class="oncall-stat">
                <span class="oncall-stat-value" id="oncall-active-count">0</span>
                <span class="oncall-stat-label">On Duty</span>
            </div>
        </div>
        <div class="oncall-current" id="oncall-current">
            <div class="oncall-current-label">Currently On-Call</div>
            <div class="oncall-current-person" id="oncall-current-person">
                <div class="empty-state" style="padding:0.5rem 0;">No schedules configured</div>
            </div>
        </div>
        <div class="widget-body no-pad" id="oncall-schedules-list" style="overflow:auto;">
            <div class="empty-state">No on-call schedules</div>
        </div>`;
}

function widgetAudit() {
    return `<div class="widget-header">
            <span class="widget-title">Audit Log</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="exportAuditLogs()">Export</button>
                <button class="btn" onclick="loadAuditWidget()">Refresh</button>
            </div>
        </div>
        <div class="audit-summary" id="audit-summary">
            <div class="audit-stat">
                <span class="audit-stat-value" id="audit-total">0</span>
                <span class="audit-stat-label">Total</span>
            </div>
            <div class="audit-stat">
                <span class="audit-stat-value" id="audit-today">0</span>
                <span class="audit-stat-label">Today</span>
            </div>
            <div class="audit-stat">
                <span class="audit-stat-value failures" id="audit-failures">0</span>
                <span class="audit-stat-label">Failures (24h)</span>
            </div>
        </div>
        <div class="audit-filters">
            <select id="audit-action-filter" onchange="loadAuditWidget()">
                <option value="">All Actions</option>
                <option value="create">Create</option>
                <option value="update">Update</option>
                <option value="delete">Delete</option>
                <option value="login">Login</option>
                <option value="logout">Logout</option>
            </select>
            <select id="audit-resource-filter" onchange="loadAuditWidget()">
                <option value="">All Resources</option>
                <option value="user">Users</option>
                <option value="dashboard">Dashboards</option>
                <option value="alert_rule">Alert Rules</option>
                <option value="notify_channel">Notifications</option>
                <option value="incident">Incidents</option>
                <option value="schedule">Schedules</option>
                <option value="session">Sessions</option>
            </select>
        </div>
        <div class="widget-body no-pad" id="audit-list" style="overflow:auto;">
            <div class="empty-state">No audit logs available</div>
        </div>`;
}

function widgetCostIntel() {
    return `<div class="widget-header">
            <span class="widget-title">Cost Intelligence</span>
            <div style="display:flex;gap:0.3rem;">
                <select id="cost-period" class="trace-select" style="height:26px;" onchange="loadCostIntel()">
                    <option value="current">Current Usage</option>
                    <option value="projected">Projected (30d)</option>
                </select>
                <button class="btn" onclick="loadCostIntel()">Refresh</button>
            </div>
        </div>
        <div class="widget-body" id="cost-intel-content">
            <div class="empty-state">Loading cost estimates...</div>
        </div>`;
}

function widgetDBWatch() {
    return `<div class="widget-header">
            <span class="widget-title">Database Queries</span>
            <div style="display:flex;gap:0.3rem;">
                <select id="dbwatch-db-filter" class="trace-select" style="height:26px;" onchange="loadDBWatch()">
                    <option value="">All Databases</option>
                    <option value="redis">Redis</option>
                    <option value="postgres">PostgreSQL</option>
                    <option value="mysql">MySQL</option>
                </select>
                <button class="btn" onclick="loadDBWatch()">Refresh</button>
            </div>
        </div>
        <div class="dbwatch-tabs">
            <button class="dbwatch-tab active" data-tab="queries" onclick="switchDBWatchTab('queries')">Recent Queries</button>
            <button class="dbwatch-tab" data-tab="slow" onclick="switchDBWatchTab('slow')">Slow Queries</button>
            <button class="dbwatch-tab" data-tab="stats" onclick="switchDBWatchTab('stats')">Stats</button>
        </div>
        <div class="widget-body no-pad" id="dbwatch-content" style="overflow:auto;">
            <div class="empty-state">Loading database queries...</div>
        </div>`;
}

function widgetCardinality() {
    return `<div class="widget-header">
            <span class="widget-title">Cardinality Explorer</span>
            <div style="display:flex;gap:0.3rem;">
                <button class="btn" onclick="loadCardinality()">Refresh</button>
            </div>
        </div>
        <div class="widget-body" id="cardinality-content" style="overflow:auto;">
            <div class="empty-state">Loading cardinality data...</div>
        </div>`;
}


// Initialize GridStack grid
function initGrid() {
    grid = GridStack.init({
        column: 12,
        cellHeight: 70,
        margin: 8,
        float: false,
        animate: true,
        draggable: { handle: '.widget-header' },
        resizable: { handles: 'se,e,s' },
        minRow: 1
    });
}

function loadLayout() {
    const saved = localStorage.getItem('dogwatch-layout');
    let items = saved ? JSON.parse(saved) : defaultLayout;

    // Merge with defaults to ensure minW/minH constraints are preserved
    const defaultsById = {};
    defaultLayout.forEach(d => defaultsById[d.id] = d);

    items = items.map(item => {
        const defaults = defaultsById[item.id] || {};
        return {
            ...defaults,
            ...item,
            content: `<div class="gs-item-content">${getWidgetContent(item.id)}</div>`
        };
    });

    grid.load(items);
}

function getWidgetContent(id) {
    const map = {
        cpu: widgetCPU, mem: widgetMem, disk: widgetDisk, net: widgetNet,
        conns: () => widgetStat('Connections', 'total-connections'),
        reqs: () => widgetStat('Requests', 'total-requests'),
        errs: () => widgetStat('Errors', 'total-errors', true),
        svcmap: widgetServiceMap,
        traces: widgetTraces,
        watches: widgetWatches,
        logs: widgetLogs,
        synthetics: widgetSynthetics,
        slos: widgetSLOs,
        patterns: widgetPatterns,
        containers: widgetContainers,
        deployments: widgetDeployments,
        incidents: widgetIncidents,
        cluster: widgetCluster,
        kubernetes: widgetKubernetes,
        anomaly: widgetAnomaly,
        alerting: widgetAlerting,
        notifications: widgetNotifications,
        oncall: widgetOnCall,
        audit: widgetAudit,
        statuspage: widgetStatusPage,
        catalog: widgetCatalog,
        correlation: widgetCorrelation,
        cpuchart: () => widgetChart('CPU History', 'cpu-chart'),
        memchart: () => widgetChart('Memory History', 'mem-chart'),
        endpoints: widgetEndpoints,
        connlist: widgetConnections,
        procs: widgetProcesses,
        flamegraph: widgetFlameGraph,
        costintel: widgetCostIntel,
        dbwatch: widgetDBWatch,
        cardinality: widgetCardinality
    };
    return map[id] ? map[id]() : '';
}

function saveLayout() {
    const items = grid.save(false);
    localStorage.setItem('dogwatch-layout', JSON.stringify(items));
}

function resetLayout() {
    localStorage.removeItem('dogwatch-layout');
    currentDashboardId = null;
    document.getElementById('dashboard-select').value = '';
    grid.removeAll();
    loadLayout();
    setTimeout(initCharts, 100);
}

// Dashboard persistence
// moved to top
// moved to top

async function loadDashboards() {
    try {
        const resp = await fetch('/api/dashboards');
        dashboards = await resp.json() || [];
        const select = document.getElementById('dashboard-select');
        select.innerHTML = '<option value="">Default Layout</option>';
        dashboards.forEach(d => {
            const opt = document.createElement('option');
            opt.value = d.id;
            opt.textContent = d.name + (d.is_default ? ' *' : '');
            select.appendChild(opt);
        });
        // Load default dashboard if exists
        const defaultDash = dashboards.find(d => d.is_default);
        if (defaultDash && !localStorage.getItem('dogwatch-layout')) {
            currentDashboardId = defaultDash.id;
            select.value = defaultDash.id;
            applyDashboardLayout(defaultDash.layout);
        }
    } catch (e) { console.error('Failed to load dashboards:', e); }
}

function applyDashboardLayout(layout) {
    if (!layout || !layout.length) return;
    grid.removeAll();
    layout.forEach(pos => {
        const widgetDef = defaultLayout.find(w => w.id === pos.id);
        if (widgetDef) {
            const content = getWidgetContent(pos.id);
            grid.addWidget({
                id: pos.id,
                x: pos.x, y: pos.y, w: pos.w, h: pos.h,
                minW: widgetDef.minW, minH: widgetDef.minH,
                content: content
            });
        }
    });
    setTimeout(initCharts, 100);
}

async function loadDashboard(id) {
    if (!id) {
        currentDashboardId = null;
        resetLayout();
        return;
    }
    try {
        const resp = await fetch(`/api/dashboards/${id}`);
        const dash = await resp.json();
        currentDashboardId = id;
        applyDashboardLayout(dash.layout);
    } catch (e) { alert('Failed to load dashboard'); }
}

async function saveDashboard() {
    const layout = grid.save(false).map(item => ({
        id: item.id, x: item.x, y: item.y, w: item.w, h: item.h
    }));

    if (currentDashboardId) {
        // Update existing dashboard
        const dash = dashboards.find(d => d.id === currentDashboardId);
        const name = prompt('Dashboard name:', dash?.name || 'My Dashboard');
        if (!name) return;
        try {
            await fetch(`/api/dashboards/${currentDashboardId}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name, layout })
            });
            loadDashboards();
        } catch (e) { alert('Failed to save dashboard'); }
    } else {
        // Create new dashboard
        const name = prompt('Dashboard name:', 'My Dashboard');
        if (!name) return;
        try {
            const resp = await fetch('/api/dashboards', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name, layout, is_default: false })
            });
            const dash = await resp.json();
            currentDashboardId = dash.id;
            loadDashboards();
            document.getElementById('dashboard-select').value = dash.id;
        } catch (e) { alert('Failed to create dashboard'); }
    }
}

function showDashboardManager() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Manage Dashboards</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div id="dashboard-list" style="max-height: 300px; overflow-y: auto;"></div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
    renderDashboardList();
}

function renderDashboardList() {
    const list = document.getElementById('dashboard-list');
    if (!dashboards.length) {
        list.innerHTML = '<p style="color: #71767b; text-align: center; padding: 1rem;">No saved dashboards</p>';
        return;
    }
    list.innerHTML = dashboards.map(d => `
        <div style="display: flex; justify-content: space-between; align-items: center; padding: 0.6rem 0; border-bottom: 1px solid #2f3336;">
            <div>
                <div style="font-weight: 500;">${escapeHtml(d.name)}</div>
                <div style="font-size: 0.7rem; color: #71767b;">${d.is_default ? 'Default' : ''}</div>
            </div>
            <div style="display: flex; gap: 0.3rem;">
                <button class="btn" onclick="setDefaultDashboard('${d.id}')" ${d.is_default ? 'disabled' : ''}>Set Default</button>
                <button class="btn" style="background: #4a1919; color: #f4212e;" onclick="deleteDashboard('${d.id}')">Delete</button>
            </div>
        </div>
    `).join('');
}

async function setDefaultDashboard(id) {
    try {
        await fetch(`/api/dashboards/${id}/default`, { method: 'POST' });
        await loadDashboards();
        renderDashboardList();
    } catch (e) { alert('Failed to set default'); }
}

async function deleteDashboard(id) {
    if (!confirm('Delete this dashboard?')) return;
    try {
        await fetch(`/api/dashboards/${id}`, { method: 'DELETE' });
        if (currentDashboardId === id) {
            currentDashboardId = null;
            document.getElementById('dashboard-select').value = '';
        }
        await loadDashboards();
        renderDashboardList();
    } catch (e) { alert('Failed to delete dashboard'); }
}

grid.on('change', saveLayout);
loadLayout();
loadDashboards();
setTimeout(initCharts, 100);

// Utility functions
function formatBytes(b) {
    if (b === 0) return '0 B';
    const k = 1024, s = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(b) / Math.log(k));
    return (b / Math.pow(k, i)).toFixed(1) + ' ' + s[i];
}
function formatBPS(b) { return formatBytes(b) + '/s'; }
function formatLatency(ms) {
    if (!ms) return '-';
    return ms < 1 ? '<1ms' : ms < 1000 ? Math.round(ms) + 'ms' : (ms/1000).toFixed(1) + 's';
}
function getLatencyClass(ms) { return !ms ? '' : ms < 100 ? 'good' : ms < 500 ? 'warn' : 'bad'; }
function escapeHtml(t) { const d = document.createElement('div'); d.textContent = t; return d.innerHTML; }

// Demo/Simulated Data Generator (reassigns the placeholder from top)
DemoData = {
    enabled: true, // Set to true to show demo data when real data is empty

    services: ['api-gateway', 'user-service', 'payment-service', 'inventory-service', 'notification-service', 'auth-service', 'search-service', 'order-service'],

    generateServiceMap() {
        const nodes = this.services.map((name, i) => ({
            id: name,
            name: name,
            type: i < 6 ? 'service' : 'external',
            status: i === 2 ? 'degraded' : (i === 5 ? 'error' : 'healthy'),
            requests: Math.floor(Math.random() * 10000) + 1000,
            latency: Math.floor(Math.random() * 200) + 20,
            errors: i === 5 ? Math.floor(Math.random() * 50) + 10 : Math.floor(Math.random() * 5)
        }));
        const links = [
            { source: 'api-gateway', target: 'user-service', count: 5420, latency: 45, errors: 2 },
            { source: 'api-gateway', target: 'auth-service', count: 8900, latency: 23, errors: 45 },
            { source: 'api-gateway', target: 'order-service', count: 3200, latency: 89, errors: 5 },
            { source: 'user-service', target: 'notification-service', count: 1200, latency: 120, errors: 0 },
            { source: 'order-service', target: 'payment-service', count: 2800, latency: 156, errors: 12 },
            { source: 'order-service', target: 'inventory-service', count: 3100, latency: 67, errors: 3 },
            { source: 'payment-service', target: 'notification-service', count: 890, latency: 89, errors: 1 },
            { source: 'inventory-service', target: 'search-service', count: 4500, latency: 34, errors: 0 }
        ];
        return { nodes, links };
    },

    generateTraces() {
        const now = Date.now();
        return Array.from({ length: 15 }, (_, i) => ({
            trace_id: `trace-${Date.now()}-${i}`,
            name: ['GET /api/users', 'POST /api/orders', 'GET /api/products', 'POST /api/payments', 'GET /api/search'][i % 5],
            service_name: this.services[i % this.services.length],
            span_count: Math.floor(Math.random() * 8) + 2,
            duration_ms: Math.floor(Math.random() * 500) + 20,
            status: i === 3 ? 'error' : (i === 7 ? 'error' : 'ok'),
            start_time: new Date(now - i * 60000).toISOString()
        }));
    },

    generateTraceDetail(traceId) {
        const now = Date.now();
        const services = ['api-gateway', 'user-service', 'database'];
        return {
            trace_id: traceId,
            spans: [
                { span_id: 'span-1', parent_span_id: null, name: 'GET /api/users', service_name: 'api-gateway', kind: 'server', start_time: new Date(now - 200).toISOString(), end_time: new Date(now).toISOString(), status: 'ok', attributes: { 'http.method': 'GET', 'http.url': '/api/users', 'http.status_code': 200 } },
                { span_id: 'span-2', parent_span_id: 'span-1', name: 'user.getAll', service_name: 'user-service', kind: 'internal', start_time: new Date(now - 180).toISOString(), end_time: new Date(now - 50).toISOString(), status: 'ok', attributes: { 'code.function': 'getAll' } },
                { span_id: 'span-3', parent_span_id: 'span-2', name: 'SELECT * FROM users', service_name: 'database', kind: 'client', start_time: new Date(now - 160).toISOString(), end_time: new Date(now - 80).toISOString(), status: 'ok', attributes: { 'db.system': 'postgresql', 'db.statement': 'SELECT * FROM users LIMIT 100' } },
                { span_id: 'span-4', parent_span_id: 'span-1', name: 'cache.check', service_name: 'api-gateway', kind: 'client', start_time: new Date(now - 195).toISOString(), end_time: new Date(now - 185).toISOString(), status: 'ok', attributes: { 'cache.hit': 'false' } }
            ]
        };
    },

    generateLogs() {
        const now = Date.now();
        const levels = ['info', 'info', 'info', 'warn', 'error', 'info', 'debug'];
        const messages = [
            'Request processed successfully',
            'User authentication completed',
            'Database connection established',
            'High memory usage detected: 85%',
            'Failed to connect to payment gateway: timeout',
            'Cache hit ratio: 94.2%',
            'Starting health check routine',
            'Order #12345 created successfully',
            'Rate limit exceeded for client 192.168.1.100',
            'Retrying failed request (attempt 2/3)'
        ];
        return {
            entries: Array.from({ length: 20 }, (_, i) => ({
                timestamp: new Date(now - i * 30000).toISOString(),
                level: levels[i % levels.length],
                service: this.services[i % this.services.length],
                message: messages[i % messages.length],
                trace_id: i % 4 === 0 ? `trace-demo-${i}` : null
            }))
        };
    },

    generateIncidents() {
        const now = Date.now();
        return [
            { id: 'inc-1', title: 'High error rate on payment-service', status: 'triggered', severity: 'critical', service: 'payment-service', created_at: new Date(now - 300000).toISOString(), source: 'alerting' },
            { id: 'inc-2', title: 'Database connection pool exhausted', status: 'acknowledged', severity: 'high', service: 'user-service', created_at: new Date(now - 1800000).toISOString(), assigned_to: 'alice', source: 'anomaly' },
            { id: 'inc-3', title: 'Elevated latency on search queries', status: 'resolved', severity: 'medium', service: 'search-service', created_at: new Date(now - 7200000).toISOString(), source: 'slo' },
            { id: 'inc-4', title: 'SSL certificate expiring in 7 days', status: 'triggered', severity: 'low', service: 'api-gateway', created_at: new Date(now - 86400000).toISOString(), source: 'synthetics' }
        ];
    },

    generateIncidentStats() {
        return { active_incidents: 2, triggered_count: 2, acknowledged_count: 1 };
    },

    generateDeployments() {
        const now = Date.now();
        return [
            { id: 'd1', service: 'api-gateway', version: 'v2.4.1', environment: 'prod', status: 'success', timestamp: new Date(now - 3600000).toISOString(), user: 'alice', commit_sha: 'a1b2c3d', duration_ms: 45000 },
            { id: 'd2', service: 'user-service', version: 'v1.8.0', environment: 'prod', status: 'success', timestamp: new Date(now - 7200000).toISOString(), user: 'bob', commit_sha: 'e4f5g6h', duration_ms: 32000 },
            { id: 'd3', service: 'payment-service', version: 'v3.1.2', environment: 'staging', status: 'failed', timestamp: new Date(now - 14400000).toISOString(), user: 'charlie', commit_sha: 'i7j8k9l', duration_ms: 120000 },
            { id: 'd4', service: 'order-service', version: 'v2.0.0', environment: 'prod', status: 'success', timestamp: new Date(now - 86400000).toISOString(), user: 'alice', commit_sha: 'm0n1o2p', duration_ms: 55000 },
            { id: 'd5', service: 'notification-service', version: 'v1.2.3', environment: 'prod', status: 'rolled_back', timestamp: new Date(now - 172800000).toISOString(), user: 'bob', commit_sha: 'q3r4s5t', duration_ms: 28000 }
        ];
    },

    generateDeployStats() {
        return { total_deployments: 47, deploys_today: 3, deploys_this_week: 12, success_rate: 94.5, recent_failures: 1 };
    },

    generateSLOs() {
        return [
            { slo: { id: 'slo-1', name: 'API Availability', target: 99.9, window: '30d', type: 'availability' }, state: { status: 'OK', current_value: 99.94, budget_used_pct: 42, budget_remaining: 8.2 } },
            { slo: { id: 'slo-2', name: 'Checkout Latency P95', target: 99.0, window: '7d', type: 'latency' }, state: { status: 'WARNING', current_value: 98.7, budget_used_pct: 78, budget_remaining: 2.1 } },
            { slo: { id: 'slo-3', name: 'Search Error Rate', target: 99.5, window: '30d', type: 'error_rate' }, state: { status: 'OK', current_value: 99.82, budget_used_pct: 23, budget_remaining: 15.4 } },
            { slo: { id: 'slo-4', name: 'Payment Success Rate', target: 99.95, window: '24h', type: 'availability' }, state: { status: 'CRITICAL', current_value: 99.21, budget_used_pct: 156, budget_remaining: -2.8 } }
        ];
    },

    generateSynthetics() {
        return [
            { id: 's1', name: 'Homepage Health', url: 'https://app.example.com/health', status: 'passing', last_latency_ms: 124 },
            { id: 's2', name: 'API Status', url: 'https://api.example.com/status', status: 'passing', last_latency_ms: 45 },
            { id: 's3', name: 'Login Flow', url: 'https://app.example.com/login', status: 'failing', last_latency_ms: 5230 },
            { id: 's4', name: 'Checkout Process', url: 'https://app.example.com/checkout', status: 'passing', last_latency_ms: 890 }
        ];
    },

    generateFlameGraph() {
        return {
            name: 'root',
            value: 10000,
            children: [
                { name: 'main.handleRequest', value: 4500, children: [
                    { name: 'http.ServeHTTP', value: 3800, children: [
                        { name: 'json.Marshal', value: 1200, children: [] },
                        { name: 'db.Query', value: 2100, children: [
                            { name: 'sql.QueryContext', value: 1800, children: [
                                { name: 'runtime.gcBgMarkWorker', value: 400, children: [] }
                            ]}
                        ]}
                    ]},
                    { name: 'middleware.Auth', value: 600, children: [] }
                ]},
                { name: 'runtime.schedule', value: 2000, children: [
                    { name: 'runtime.findrunnable', value: 1200, children: [] },
                    { name: 'runtime.gcDrain', value: 700, children: [] }
                ]},
                { name: 'net.(*netFD).Read', value: 1500, children: [
                    { name: 'syscall.read', value: 1400, children: [
                        { name: '[kernel]', value: 800, children: [] }
                    ]}
                ]},
                { name: 'main.processQueue', value: 2000, children: [
                    { name: 'encoding/json.Unmarshal', value: 900, children: [] },
                    { name: 'kafka.Consume', value: 1000, children: [] }
                ]}
            ]
        };
    },

    generateAnomalies() {
        const now = Date.now();
        return [
            { metric_name: 'payment_service.latency_p99', value: 450.5, score: 0.92, is_critical: true, timestamp: new Date(now - 120000).toISOString(), reason: 'Value 3.2x above normal baseline' },
            { metric_name: 'api_gateway.error_rate', value: 2.4, score: 0.78, is_critical: false, timestamp: new Date(now - 300000).toISOString(), reason: 'Unusual spike detected' },
            { metric_name: 'user_service.memory_usage', value: 87.3, score: 0.65, is_critical: false, timestamp: new Date(now - 600000).toISOString(), reason: 'Gradual increase over 30m' },
            { metric_name: 'search_service.query_count', value: 12500, score: 0.45, is_critical: false, timestamp: new Date(now - 1800000).toISOString(), reason: 'Higher than expected for this time' }
        ];
    },

    generateAnomalyStats() {
        return { critical_count: 1, warning_count: 2, total_anomalies: 4, metrics_tracked: 127 };
    },

    generateOnCallSchedules() {
        return [
            { id: 'oc1', name: 'Platform Team', rotation_type: 'weekly', users: [
                { user_id: 'u1', name: 'Alice Chen', email: 'alice@example.com' },
                { user_id: 'u2', name: 'Bob Smith', email: 'bob@example.com' },
                { user_id: 'u3', name: 'Charlie Davis', email: 'charlie@example.com' }
            ]},
            { id: 'oc2', name: 'Database Team', rotation_type: 'daily', users: [
                { user_id: 'u4', name: 'Diana Lee', email: 'diana@example.com' },
                { user_id: 'u5', name: 'Eve Wilson', email: 'eve@example.com' }
            ]}
        ];
    },

    generateAlerts() {
        const now = Date.now();
        return [
            { name: 'HighErrorRate', state: 'firing', severity: 'critical', starts_at: new Date(now - 600000).toISOString(), labels: { service: 'payment-service', env: 'prod' }, annotations: { summary: 'Error rate above 5%' } },
            { name: 'HighLatency', state: 'firing', severity: 'warning', starts_at: new Date(now - 1200000).toISOString(), labels: { service: 'search-service', env: 'prod' }, annotations: { summary: 'P99 latency above 500ms' } }
        ];
    },

    generateAlertingStatus() {
        return { firing_alerts: 2, pending_alerts: 1, active_silences: 0, total_rules: 24 };
    },

    generateCurrentOnCall() {
        return { 'Platform Team': 'Alice Chen', 'Database Team': 'Diana Lee' };
    },

    generatePatterns() {
        const now = Date.now();
        return {
            stats: { total_patterns: 12, total_matches: 45230, matches_last_hour: 892, new_patterns_today: 3 },
            patterns: [
                { signature: 'Error connecting to database: connection refused', level: 'error', service: 'user-service', count: 1247, count_last_hr: 45, trend: 'increasing', last_seen: new Date(now - 60000).toISOString(), examples: ['Error connecting to database: connection refused at 10.0.0.5:5432'] },
                { signature: 'Request completed successfully in {duration}ms', level: 'info', service: 'api-gateway', count: 28934, count_last_hr: 523, trend: 'stable', last_seen: new Date(now - 5000).toISOString(), examples: ['Request completed successfully in 45ms'] },
                { signature: 'Rate limit exceeded for client {ip}', level: 'warn', service: 'api-gateway', count: 892, count_last_hr: 67, trend: 'increasing', last_seen: new Date(now - 120000).toISOString(), examples: ['Rate limit exceeded for client 192.168.1.100'] },
                { signature: 'Cache miss for key {key}', level: 'debug', service: 'search-service', count: 5621, count_last_hr: 234, trend: 'stable', last_seen: new Date(now - 30000).toISOString(), examples: ['Cache miss for key user:12345'] },
                { signature: 'Payment processed: {amount} {currency}', level: 'info', service: 'payment-service', count: 3456, count_last_hr: 89, trend: 'decreasing', last_seen: new Date(now - 180000).toISOString(), examples: ['Payment processed: 99.99 USD'] },
                { signature: 'New user registered: {email}', level: 'info', service: 'auth-service', count: 234, count_last_hr: 12, trend: 'new', last_seen: new Date(now - 300000).toISOString(), examples: ['New user registered: user@example.com'] }
            ]
        };
    },

    generateCatalogServices() {
        return [
            { id: 'svc-1', name: 'api-gateway', display_name: 'API Gateway', description: 'Main entry point for all API requests', tier: 'critical', health: 'healthy', team_name: 'Platform', repo_url: 'https://github.com/example/api-gateway', docs_url: 'https://docs.example.com/api' },
            { id: 'svc-2', name: 'user-service', display_name: 'User Service', description: 'User management and authentication', tier: 'critical', health: 'degraded', team_name: 'Identity', repo_url: 'https://github.com/example/user-service' },
            { id: 'svc-3', name: 'payment-service', display_name: 'Payment Service', description: 'Payment processing and billing', tier: 'critical', health: 'unhealthy', team_name: 'Payments', runbook_url: 'https://runbooks.example.com/payments' },
            { id: 'svc-4', name: 'notification-service', display_name: 'Notification Service', description: 'Email, SMS, and push notifications', tier: 'high', health: 'healthy', team_name: 'Platform' },
            { id: 'svc-5', name: 'search-service', display_name: 'Search Service', description: 'Full-text search and indexing', tier: 'medium', health: 'healthy', team_name: 'Search' },
            { id: 'svc-6', name: 'inventory-service', display_name: 'Inventory Service', description: 'Product inventory management', tier: 'high', health: 'healthy', team_name: 'Commerce' }
        ];
    },

    generateCatalogStats() {
        return { total: 6, critical: 3, healthy: 4, unhealthy: 1, degraded: 1 };
    },

    generateCatalogTeams() {
        return [
            { id: 't1', name: 'Platform', member_count: 8 },
            { id: 't2', name: 'Identity', member_count: 5 },
            { id: 't3', name: 'Payments', member_count: 6 },
            { id: 't4', name: 'Search', member_count: 4 },
            { id: 't5', name: 'Commerce', member_count: 7 }
        ];
    },

    generateCorrelations() {
        const now = Date.now();
        return {
            correlations: [
                { deployment: { id: 'd1', service: 'payment-service', version: 'v3.1.2' }, incident: { id: 'inc-1', title: 'High error rate' }, confidence: 0.89, time_delta: 300000, reason: 'Error rate spiked 5 minutes after deploy' },
                { deployment: { id: 'd2', service: 'user-service', version: 'v1.8.0' }, incident: { id: 'inc-2', title: 'Database connection issues' }, confidence: 0.72, time_delta: 600000, reason: 'DB connections increased 10 minutes after deploy' }
            ]
        };
    },

    generateServiceTimeline() {
        const now = Date.now();
        return {
            events: [
                { type: 'deploy', timestamp: new Date(now - 3600000).toISOString(), data: { version: 'v2.4.1' } },
                { type: 'error_spike', timestamp: new Date(now - 3300000).toISOString(), data: { error_rate: 5.2 } },
                { type: 'incident', timestamp: new Date(now - 3000000).toISOString(), data: { title: 'High error rate detected' } },
                { type: 'metric_anomaly', timestamp: new Date(now - 2700000).toISOString(), data: { metric: 'latency_p99', value: 450 } }
            ],
            total_events: 4,
            error_traces: 23
        };
    },

    generateClusterInfo() {
        return {
            enabled: true,
            node_count: 3,
            node_id: 'node-local-001',
            gossip_addr: '10.0.0.1:7946',
            local_node: { id: 'node-local-001', hostname: 'dogwatch-1' }
        };
    },

    generateClusterNodes() {
        const now = Date.now();
        return [
            { id: 'node-local-001', hostname: 'dogwatch-1', address: '10.0.0.1:9999', version: '1.0.0', state: 'alive', started_at: new Date(now - 86400000).toISOString(), cpu_percent: 45.2, mem_percent: 62.1, active_incidents: 2 },
            { id: 'node-002', hostname: 'dogwatch-2', address: '10.0.0.2:9999', version: '1.0.0', state: 'alive', started_at: new Date(now - 172800000).toISOString(), cpu_percent: 32.8, mem_percent: 58.4, active_incidents: 1 },
            { id: 'node-003', hostname: 'dogwatch-3', address: '10.0.0.3:9999', version: '1.0.0', state: 'alive', started_at: new Date(now - 259200000).toISOString(), cpu_percent: 28.1, mem_percent: 54.9, active_incidents: 0 }
        ];
    },

    generateHistoryData(durationMs) {
        const now = Date.now();
        const points = 60;
        const interval = durationMs / points;
        const data = [];
        let cpuBase = 25 + Math.random() * 20;
        let memBase = 55 + Math.random() * 15;
        for (let i = 0; i < points; i++) {
            cpuBase += (Math.random() - 0.5) * 8;
            memBase += (Math.random() - 0.5) * 3;
            cpuBase = Math.max(5, Math.min(95, cpuBase));
            memBase = Math.max(40, Math.min(90, memBase));
            data.push({
                timestamp: new Date(now - durationMs + i * interval).toISOString(),
                cpu_percent: cpuBase,
                mem_percent: memBase
            });
        }
        return data;
    }
};

// Data fetching
function updateSystemMetrics() {
    fetch('/api/system').then(r => r.json()).then(d => {
        const cpu = d.cpu_usage_percent || 0;
        document.getElementById('cpu-percent').textContent = cpu.toFixed(1) + '%';
        document.getElementById('cpu-bar').style.width = cpu + '%';
        document.getElementById('cpu-iowait').textContent = (d.cpu_iowait || 0).toFixed(1) + '%';
        document.getElementById('load-avg').textContent = (d.load_1 || 0).toFixed(2);

        const mem = d.mem_usage_percent || 0;
        document.getElementById('mem-percent').textContent = mem.toFixed(1) + '%';
        document.getElementById('mem-bar').style.width = mem + '%';
        document.getElementById('mem-used').textContent = formatBytes(d.mem_used_bytes || 0);
        document.getElementById('mem-total').textContent = formatBytes(d.mem_total_bytes || 0);

        const dr = d.disk_read_per_sec || 0, dw = d.disk_write_per_sec || 0;
        document.getElementById('disk-io').textContent = formatBPS(dr + dw);
        document.getElementById('disk-read').textContent = formatBPS(dr);
        document.getElementById('disk-write').textContent = formatBPS(dw);
        document.getElementById('disk-bar').style.width = Math.min((dr + dw) / 1e8 * 100, 100) + '%';

        const nr = d.net_rx_per_sec || 0, nt = d.net_tx_per_sec || 0;
        document.getElementById('net-io').textContent = formatBPS(nr + nt);
        document.getElementById('net-rx').textContent = formatBPS(nr);
        document.getElementById('net-tx').textContent = formatBPS(nt);
        document.getElementById('net-bar').style.width = Math.min((nr + nt) / 1e7 * 100, 100) + '%';
    }).catch(() => {});
}

function updateStats() {
    fetch('/api/stats').then(r => r.json()).then(d => {
        document.getElementById('total-connections').textContent = (d.total_connections || 0).toLocaleString();
        document.getElementById('total-requests').textContent = (d.total_requests || 0).toLocaleString();
        document.getElementById('total-errors').textContent = (d.total_errors || 0).toLocaleString();
        document.getElementById('last-update').textContent = new Date().toLocaleTimeString();

        const eb = document.getElementById('endpoints-body');
        if (d.endpoints?.length) {
            eb.innerHTML = d.endpoints.slice(0, 10).map(e => `<tr>
                <td><span class="method ${e.method}">${e.method}</span></td>
                <td class="path">${escapeHtml(e.path)}</td>
                <td>${e.request_count}</td>
                <td>${e.error_rate.toFixed(1)}%</td>
                <td class="latency ${getLatencyClass(e.p50_ms)}">${formatLatency(e.p50_ms)}</td>
                <td class="latency ${getLatencyClass(e.p99_ms)}">${formatLatency(e.p99_ms)}</td>
            </tr>`).join('');
        }

        const cb = document.getElementById('connections-body');
        if (d.connections?.length) {
            cb.innerHTML = d.connections.slice(0, 10).map(c => `<tr>
                <td>${escapeHtml(c.process)}</td>
                <td style="font-family:monospace;font-size:0.7rem">${escapeHtml(c.remote)}:${c.port}</td>
                <td>${c.count}</td>
            </tr>`).join('');
        }
    }).catch(() => {});
}

function updateProcesses() {
    fetch('/api/processes').then(r => r.json()).then(d => {
        const pb = document.getElementById('processes-body');
        if (d?.length) {
            pb.innerHTML = d.slice(0, 10).map(p => `<tr>
                <td>${p.pid}</td>
                <td>${escapeHtml(p.name)}</td>
                <td>${p.cpu_pct.toFixed(1)}%</td>
                <td>${p.mem_mb.toFixed(0)}MB</td>
            </tr>`).join('');
        }
    }).catch(() => {});
}

// Service map - redesigned
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top

function updateServiceMap() {
    fetch('/api/servicemap').then(r => r.json()).then(d => {
        svcMapData = d || { nodes: [], links: [] };
        // Use demo data if empty and demo mode enabled
        if (DemoData.enabled && (!svcMapData.nodes || svcMapData.nodes.length === 0)) {
            svcMapData = DemoData.generateServiceMap();
        }
        renderServiceMap();
    }).catch(() => {
        if (DemoData.enabled) {
            svcMapData = DemoData.generateServiceMap();
            renderServiceMap();
        }
    });
}

function filterServiceMap() {
    svcMapFilter = document.getElementById('svcmap-filter')?.value || '';
    renderServiceMap();
}

function toggleSvcMapLayout() {
    svcMapLayout = svcMapLayout === 'hierarchical' ? 'radial' : 'hierarchical';
    svcMapTransform = d3.zoomIdentity; // Reset zoom on layout change
    renderServiceMap();
}

function renderServiceMap() {
    const container = document.getElementById('svcmap-container');
    const svg = d3.select('#service-map');
    const tooltip = document.getElementById('svcmap-tooltip');
    const statsEl = document.getElementById('svcmap-stats');

    if (!container || !svg.node()) return;
    const rect = container.getBoundingClientRect();
    if (rect.width < 50 || rect.height < 50) return;

    const w = rect.width, h = rect.height - 50;
    svg.attr('width', w).attr('height', h);

    // Filter nodes
    let nodes = svcMapData.nodes.map(n => ({...n}));
    if (svcMapFilter) {
        if (svcMapFilter === 'services') nodes = nodes.filter(n => n.type === 'service');
        else if (svcMapFilter === 'external') nodes = nodes.filter(n => n.type === 'external');
        else if (svcMapFilter === 'processes') nodes = nodes.filter(n => n.type === 'process');
    }

    const nodeIds = new Set(nodes.map(n => n.id));
    let links = svcMapData.links.filter(l => nodeIds.has(l.source?.id || l.source) && nodeIds.has(l.target?.id || l.target)).map(l => ({...l}));

    // Calculate aggregate stats
    const totalRequests = links.reduce((sum, l) => sum + (l.count || 0), 0);
    const avgLatency = links.length ? Math.round(links.reduce((sum, l) => sum + (l.latency || 0), 0) / links.length) : 0;
    const totalErrors = links.reduce((sum, l) => sum + (l.errors || 0), 0);
    const errorRate = totalRequests > 0 ? ((totalErrors / totalRequests) * 100).toFixed(1) : 0;

    if (statsEl) {
        statsEl.innerHTML = `<span>${nodes.length} nodes</span><span>·</span><span>${links.length} edges</span>` +
            (totalRequests > 0 ? `<span>·</span><span>${totalRequests} req</span><span>·</span><span>${avgLatency}ms avg</span>` : '') +
            (totalErrors > 0 ? `<span>·</span><span style="color:#f4212e">${errorRate}% err</span>` : '');
    }

    if (!nodes.length) {
        svg.selectAll('*').remove();
        svg.append('text').attr('x', w/2).attr('y', h/2).attr('text-anchor', 'middle')
            .attr('fill', '#71767b').attr('font-size', '14px').text('No service connections yet');
        return;
    }

    svg.selectAll('*').remove();

    // Defs for markers and filters (add before main group)
    const defs = svg.append('defs');

    // Arrow markers with different colors
    ['#536471', '#1d9bf0', '#f4212e', '#00ba7c'].forEach((color, i) => {
        defs.append('marker')
            .attr('id', `arrow-${i}`).attr('viewBox', '0 -5 10 10')
            .attr('refX', 32).attr('refY', 0)
            .attr('markerWidth', 5).attr('markerHeight', 5).attr('orient', 'auto')
            .append('path').attr('d', 'M0,-4L10,0L0,4').attr('fill', color);
    });

    // Glow filter for selected nodes
    const glow = defs.append('filter').attr('id', 'glow').attr('x', '-50%').attr('y', '-50%').attr('width', '200%').attr('height', '200%');
    glow.append('feGaussianBlur').attr('stdDeviation', '3').attr('result', 'blur');
    glow.append('feMerge').html('<feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/>');

    // Main group for zoom/pan
    svcMapMainGroup = svg.append('g').attr('class', 'svcmap-main-group');

    // Setup zoom behavior - instant transforms, no transitions
    svcMapZoomBehavior = d3.zoom()
        .scaleExtent([0.2, 4])
        .filter((event) => {
            // Allow wheel zoom and drag pan, but not on double-click
            return !event.button && event.type !== 'dblclick';
        })
        .on('zoom', (event) => {
            svcMapTransform = event.transform;
            svcMapMainGroup.attr('transform', event.transform);
        });

    // Initialize transform if not set
    if (!svcMapTransform) svcMapTransform = d3.zoomIdentity;

    svg.call(svcMapZoomBehavior)
       .call(svcMapZoomBehavior.transform, svcMapTransform); // Apply saved transform

    // Use force-directed layout for organic, pleasing arrangement
    const centerX = w / 2, centerY = h / 2;

    // Find the main gateway/entry node (most connections or named 'gateway')
    const connectionCount = {};
    links.forEach(l => {
        const src = l.source?.id || l.source;
        const tgt = l.target?.id || l.target;
        connectionCount[src] = (connectionCount[src] || 0) + 1;
        connectionCount[tgt] = (connectionCount[tgt] || 0) + 1;
    });

    const centralNode = nodes.reduce((best, n) => {
        const isGateway = n.id.includes('gateway') || n.id.includes('api');
        const score = (connectionCount[n.id] || 0) + (isGateway ? 10 : 0);
        return score > (best.score || 0) ? { node: n, score } : best;
    }, {}).node;

    // Position central node at center, others in a pleasing arrangement
    if (centralNode) {
        centralNode.x = centerX;
        centralNode.y = centerY;
        centralNode.fx = centerX; // Fixed position
        centralNode.fy = centerY;
    }

    // Initialize other nodes in a circle around center
    const otherNodes = nodes.filter(n => n !== centralNode);
    const radius = Math.min(w, h) * 0.35;
    otherNodes.forEach((n, i) => {
        const angle = (i / otherNodes.length) * 2 * Math.PI - Math.PI / 2;
        n.x = centerX + radius * Math.cos(angle);
        n.y = centerY + radius * Math.sin(angle);
    });

    // Run force simulation for smooth layout
    const simulation = d3.forceSimulation(nodes)
        .force('link', d3.forceLink(links).id(d => d.id).distance(140).strength(0.5))
        .force('charge', d3.forceManyBody().strength(-400))
        .force('center', d3.forceCenter(centerX, centerY))
        .force('collision', d3.forceCollide().radius(60))
        .stop();

    // Run simulation synchronously for instant layout
    for (let i = 0; i < 150; i++) simulation.tick();

    // Unfix central node after simulation
    if (centralNode) {
        delete centralNode.fx;
        delete centralNode.fy;
    }

    // Draw edges first (under nodes)
    const linkGroup = svcMapMainGroup.append('g').attr('class', 'svcmap-links');
    links.forEach(l => {
        const source = nodes.find(n => n.id === (l.source?.id || l.source));
        const target = nodes.find(n => n.id === (l.target?.id || l.target));
        if (!source || !target) return;

        const hasErrors = l.errors > 0;
        const isActive = l.count > 0;
        const errorRate = l.count > 0 ? (l.errors || 0) / l.count : 0;

        // Curved path with bezier
        const dx = target.x - source.x;
        const dy = target.y - source.y;
        const dist = Math.sqrt(dx * dx + dy * dy);
        const curve = Math.min(40, dist * 0.15);

        // Perpendicular offset for curve
        const mx = (source.x + target.x) / 2;
        const my = (source.y + target.y) / 2;
        const perpX = -dy / dist * curve;
        const perpY = dx / dist * curve;

        const pathData = `M${source.x},${source.y} Q${mx + perpX},${my + perpY} ${target.x},${target.y}`;

        // Determine edge style
        let edgeClass = 'svcmap-edge';
        let markerIdx = 0;
        if (hasErrors && errorRate > 0.1) {
            edgeClass += ' svcmap-edge-error';
            markerIdx = 2;
        } else if (isActive) {
            edgeClass += ' svcmap-edge-active';
            markerIdx = 1;
        }

        const edgeG = linkGroup.append('g');

        // Draw edge
        edgeG.append('path')
            .attr('d', pathData)
            .attr('class', edgeClass)
            .attr('marker-end', `url(#arrow-${markerIdx})`);

        // Edge metrics badge (shown on hover or always for important edges)
        if (l.count || l.latency) {
            const badgeX = mx + perpX * 0.5;
            const badgeY = my + perpY * 0.5;

            const badge = edgeG.append('g')
                .attr('class', 'svcmap-edge-badge')
                .attr('transform', `translate(${badgeX},${badgeY})`);

            // Badge background
            badge.append('rect')
                .attr('x', -30).attr('y', -10)
                .attr('width', 60).attr('height', 20)
                .attr('rx', 4)
                .attr('fill', hasErrors ? 'rgba(74,25,25,0.95)' : 'rgba(47,51,54,0.95)')
                .attr('stroke', hasErrors ? '#f4212e' : '#3f4346');

            // Metrics text
            let metricsText = '';
            if (l.count) metricsText += `${l.count}`;
            if (l.latency) metricsText += ` · ${l.latency}ms`;
            if (l.errors && errorRate > 0) metricsText = `${(errorRate * 100).toFixed(0)}% err`;

            badge.append('text')
                .attr('text-anchor', 'middle')
                .attr('dy', 4)
                .attr('fill', hasErrors ? '#f4212e' : '#e7e9ea')
                .attr('font-size', '9px')
                .attr('font-weight', '500')
                .text(metricsText);
        }
    });

    // Draw nodes
    const nodeGroup = svcMapMainGroup.append('g').attr('class', 'svcmap-nodes');
    nodes.forEach(n => {
        const g = nodeGroup.append('g')
            .attr('class', 'svcmap-node')
            .attr('transform', `translate(${n.x},${n.y})`)
            .attr('data-node-id', n.id)
            .style('cursor', 'pointer');

        // Determine health status
        const hasErrors = n.errors > 0;
        const isDegraded = n.latency > 500;
        const isExternal = n.type === 'external';
        const isProcess = n.type === 'process';

        let nodeColor = '#00ba7c'; // healthy
        if (hasErrors) nodeColor = '#f4212e';
        else if (isDegraded) nodeColor = '#ffd400';
        else if (isExternal) nodeColor = '#536471';
        else if (isProcess) nodeColor = '#7c3aed';

        // Outer ring (health indicator)
        g.append('circle')
            .attr('r', 24)
            .attr('fill', 'none')
            .attr('stroke', nodeColor)
            .attr('stroke-width', 2)
            .attr('opacity', 0.5);

        // Inner circle
        g.append('circle')
            .attr('r', 18)
            .attr('fill', '#1a1f2e')
            .attr('stroke', nodeColor)
            .attr('stroke-width', 2);

        // Icon (using simple text icons)
        const icons = {
            'external': '⬡',
            'process': '◎',
            'service': '◆',
            'database': '▣',
            'cache': '◈',
            'queue': '▤'
        };
        g.append('text')
            .attr('text-anchor', 'middle')
            .attr('dy', 5)
            .attr('fill', nodeColor)
            .attr('font-size', '14px')
            .text(icons[n.type] || icons.service);

        // Label
        const displayName = n.name.length > 18 ? n.name.slice(0, 16) + '..' : n.name;
        g.append('text')
            .attr('class', 'svcmap-node-label')
            .attr('text-anchor', 'middle')
            .attr('y', 36)
            .attr('fill', '#e7e9ea')
            .attr('font-size', '11px')
            .attr('font-weight', '500')
            .text(displayName);

        // Metrics sublabel
        if (n.count || n.latency) {
            const metrics = [];
            if (n.count) metrics.push(`${n.count} req`);
            if (n.latency) metrics.push(`${n.latency}ms`);
            g.append('text')
                .attr('class', 'svcmap-node-sublabel')
                .attr('text-anchor', 'middle')
                .attr('y', 48)
                .attr('fill', '#71767b')
                .attr('font-size', '9px')
                .text(metrics.join(' · '));
        }

        // Error indicator badge
        if (hasErrors) {
            g.append('circle')
                .attr('cx', 14).attr('cy', -14)
                .attr('r', 8)
                .attr('fill', '#f4212e');
            g.append('text')
                .attr('x', 14).attr('y', -11)
                .attr('text-anchor', 'middle')
                .attr('fill', '#fff')
                .attr('font-size', '9px')
                .attr('font-weight', 'bold')
                .text(n.errors > 99 ? '99+' : n.errors);
        }

        // Hover effects
        g.on('mouseenter', function(event) {
            d3.select(this).select('circle:nth-child(2)').attr('filter', 'url(#glow)');

            tooltip.innerHTML = `
                <div class="svcmap-tooltip-title">${escapeHtml(n.name)}</div>
                <div class="svcmap-tooltip-row"><span>Type</span><span class="svcmap-tooltip-value">${n.type}</span></div>
                <div class="svcmap-tooltip-row"><span>Requests</span><span class="svcmap-tooltip-value">${n.count || 0}</span></div>
                ${n.latency ? `<div class="svcmap-tooltip-row"><span>Avg Latency</span><span class="svcmap-tooltip-value">${n.latency}ms</span></div>` : ''}
                ${n.errors ? `<div class="svcmap-tooltip-row"><span>Errors</span><span class="svcmap-tooltip-value" style="color:#f4212e">${n.errors}</span></div>` : ''}
            `;
            tooltip.style.display = 'block';

            const svgRect = svg.node().getBoundingClientRect();
            tooltip.style.left = Math.min(event.clientX - svgRect.left + 15, w - 180) + 'px';
            tooltip.style.top = Math.min(event.clientY - svgRect.top + 15, h - 100) + 'px';
        })
        .on('mouseleave', function() {
            d3.select(this).select('circle:nth-child(2)').attr('filter', null);
            tooltip.style.display = 'none';
        })
        .on('click', function() {
            openSvcMapDetail(n);
        });
    });
}

// Zoom controls
function svcMapZoom(factor) {
    const svg = d3.select('#service-map');
    if (svcMapZoomBehavior) {
        svg.transition().duration(200).call(svcMapZoomBehavior.scaleBy, factor);
    }
}

function svcMapReset() {
    const svg = d3.select('#service-map');
    if (svcMapZoomBehavior) {
        svcMapTransform = d3.zoomIdentity;
        svg.transition().duration(300).call(svcMapZoomBehavior.transform, d3.zoomIdentity);
    }
}

// Node detail panel
function openSvcMapDetail(node) {
    svcMapSelectedNode = node;
    const detail = document.getElementById('svcmap-detail');
    const title = document.getElementById('svcmap-detail-title');
    const body = document.getElementById('svcmap-detail-body');

    if (!detail) return;

    title.textContent = node.name;

    // Find connected nodes
    const inbound = svcMapData.links.filter(l => (l.target?.id || l.target) === node.id);
    const outbound = svcMapData.links.filter(l => (l.source?.id || l.source) === node.id);

    const inboundTotal = inbound.reduce((sum, l) => sum + (l.count || 0), 0);
    const outboundTotal = outbound.reduce((sum, l) => sum + (l.count || 0), 0);
    const errorTotal = node.errors || 0;
    const errorRate = (inboundTotal + outboundTotal) > 0 ?
        ((errorTotal / (inboundTotal + outboundTotal)) * 100).toFixed(1) : 0;

    body.innerHTML = `
        <div class="svcmap-detail-section">
            <div class="svcmap-detail-section-title">Overview</div>
            <div class="svcmap-detail-grid">
                <div class="svcmap-detail-stat">
                    <div class="svcmap-detail-stat-value">${node.count || 0}</div>
                    <div class="svcmap-detail-stat-label">Total Requests</div>
                </div>
                <div class="svcmap-detail-stat">
                    <div class="svcmap-detail-stat-value">${node.latency || 0}ms</div>
                    <div class="svcmap-detail-stat-label">Avg Latency</div>
                </div>
                <div class="svcmap-detail-stat">
                    <div class="svcmap-detail-stat-value" style="color:${errorTotal > 0 ? '#f4212e' : '#e7e9ea'}">${errorTotal}</div>
                    <div class="svcmap-detail-stat-label">Errors</div>
                </div>
                <div class="svcmap-detail-stat">
                    <div class="svcmap-detail-stat-value">${errorRate}%</div>
                    <div class="svcmap-detail-stat-label">Error Rate</div>
                </div>
            </div>
        </div>
        ${inbound.length > 0 ? `
        <div class="svcmap-detail-section">
            <div class="svcmap-detail-section-title">Inbound (${inbound.length})</div>
            <ul class="svcmap-detail-list">
                ${inbound.slice(0, 5).map(l => {
                    const srcNode = svcMapData.nodes.find(n => n.id === (l.source?.id || l.source));
                    return `<li><span>${escapeHtml(srcNode?.name || 'unknown')}</span><span>${l.count || 0} req</span></li>`;
                }).join('')}
                ${inbound.length > 5 ? `<li style="color:#71767b"><span>+${inbound.length - 5} more</span></li>` : ''}
            </ul>
        </div>` : ''}
        ${outbound.length > 0 ? `
        <div class="svcmap-detail-section">
            <div class="svcmap-detail-section-title">Outbound (${outbound.length})</div>
            <ul class="svcmap-detail-list">
                ${outbound.slice(0, 5).map(l => {
                    const tgtNode = svcMapData.nodes.find(n => n.id === (l.target?.id || l.target));
                    return `<li><span>${escapeHtml(tgtNode?.name || 'unknown')}</span><span>${l.count || 0} req</span></li>`;
                }).join('')}
                ${outbound.length > 5 ? `<li style="color:#71767b"><span>+${outbound.length - 5} more</span></li>` : ''}
            </ul>
        </div>` : ''}
    `;

    detail.classList.add('open');
}

function closeSvcMapDetail() {
    svcMapSelectedNode = null;
    const detail = document.getElementById('svcmap-detail');
    if (detail) detail.classList.remove('open');
}

function viewServiceLogs() {
    if (!svcMapSelectedNode) return;
    closeSvcMapDetail();
    showTab('logs');
    // Set service filter if available
    setTimeout(() => {
        const serviceFilter = document.getElementById('log-service-filter');
        if (serviceFilter) {
            serviceFilter.value = svcMapSelectedNode.name;
            loadLogs();
        }
    }, 100);
}

function viewServiceTraces() {
    if (!svcMapSelectedNode) return;
    closeSvcMapDetail();
    showTab('traces');
    setTimeout(() => {
        const serviceFilter = document.getElementById('trace-service-filter');
        if (serviceFilter) {
            serviceFilter.value = svcMapSelectedNode.name;
            loadTraces();
        }
    }, 100);
}

// Charts
function initCharts() {
    // Ensure Chart.js is loaded
    if (typeof Chart === 'undefined') {
        console.log('Chart.js not loaded yet, retrying...');
        setTimeout(initCharts, 500);
        return;
    }

    const cpuCanvas = document.getElementById('cpu-chart');
    const memCanvas = document.getElementById('mem-chart');

    try {
        if (cpuCanvas && !cpuChart) {
            cpuChart = new Chart(cpuCanvas.getContext('2d'), {
                type: 'line',
                data: { datasets: [{ data: [], borderColor: '#1d9bf0', backgroundColor: 'rgba(29,155,240,0.1)', fill: true, tension: 0.3 }] },
                options: chartOpts
            });
        }
        if (memCanvas && !memChart) {
            memChart = new Chart(memCanvas.getContext('2d'), {
                type: 'line',
                data: { datasets: [{ data: [], borderColor: '#00ba7c', backgroundColor: 'rgba(0,186,124,0.1)', fill: true, tension: 0.3 }] },
                options: {...chartOpts}
            });
        }
    } catch (err) {
        console.error('Chart init error:', err);
    }

    // Time button handlers
    document.querySelectorAll('.time-btn').forEach(btn => {
        btn.onclick = function() {
            this.parentElement.querySelectorAll('.time-btn').forEach(b => b.classList.remove('active'));
            this.classList.add('active');
            loadChartData(this.dataset.dur);
        };
    });
    loadChartData('15m');
}

function loadChartData(dur) {
    const durMs = dur.includes('h') ? parseInt(dur)*3600000 : parseInt(dur)*60000;
    fetch(`/api/history/system?duration=${dur}`).then(r => r.json()).then(data => {
        console.log('Chart data loaded:', data?.length, 'points');
        // Use demo data if empty and demo mode enabled
        if ((!data?.length) && DemoData.enabled) {
            data = DemoData.generateHistoryData(durMs);
        }
        if (!data?.length) {
            console.log('No chart data available');
            return;
        }
        updateCharts(data, durMs);
    }).catch(err => {
        console.error('Chart data fetch error:', err);
        if (DemoData.enabled) {
            const data = DemoData.generateHistoryData(durMs);
            updateCharts(data, durMs);
        }
    });
}

function updateCharts(data, durMs) {
    console.log('updateCharts called, cpuChart:', !!cpuChart, 'memChart:', !!memChart);
    const now = Date.now();
    if (cpuChart) {
        cpuChart.data.datasets[0].data = data.map(d => ({ x: new Date(d.timestamp), y: d.cpu_percent }));
        cpuChart.options.scales.x.min = now - durMs;
        cpuChart.options.scales.x.max = now;
        cpuChart.update('none');
    } else {
        console.log('cpuChart not initialized');
    }
    if (memChart) {
        memChart.data.datasets[0].data = data.map(d => ({ x: new Date(d.timestamp), y: d.mem_percent }));
        memChart.options.scales.x.min = now - durMs;
        memChart.options.scales.x.max = now;
        memChart.update('none');
    } else {
        console.log('memChart not initialized');
    }
}

// Traces
// moved to top
// moved to top

function loadTraceServices() {
    fetch('/api/trace/services').then(r => r.json()).then(services => {
        traceServices = services || [];
        const select = document.getElementById('trace-service-filter');
        if (select) {
            const current = select.value;
            select.innerHTML = '<option value="">All Services</option>' +
                traceServices.map(s => `<option value="${escapeHtml(s)}">${escapeHtml(s)}</option>`).join('');
            select.value = current;
        }
    }).catch(() => {});
}

function loadTraces() {
    const service = document.getElementById('trace-service-filter')?.value || '';
    const url = `/api/traces?limit=50&duration=1h${service ? '&service=' + encodeURIComponent(service) : ''}`;

    fetch(url).then(r => r.json()).then(result => {
        const list = document.getElementById('trace-list');
        if (!list) return;

        // Handle both {data: [...]} and direct array responses
        let traces = result?.data || result || [];

        // Use demo data if empty and demo mode enabled
        if ((!traces?.length) && DemoData.enabled) {
            traces = DemoData.generateTraces();
        }
        if (!traces?.length) {
            list.innerHTML = '<div class="empty-state">No traces yet. Configure OTLP export to http://localhost:9999/v1/traces</div>';
            return;
        }

        list.innerHTML = traces.map(t => {
            const statusClass = (t.status || '').toLowerCase();
            const time = new Date(t.start_time).toLocaleTimeString();
            return `<div class="trace-item ${t.trace_id === selectedTraceId ? 'selected' : ''}" data-trace-id="${t.trace_id}" onclick="selectTrace('${t.trace_id}')">
                <div>
                    <span class="trace-status ${statusClass}"></span>
                    <span class="trace-name">${escapeHtml(t.name || 'unknown')}</span>
                    <div class="trace-service">${escapeHtml(t.service_name || 'unknown')} (${t.span_count} spans)</div>
                </div>
                <div class="trace-meta">
                    <div class="trace-duration">${formatTraceDuration(t.duration_ms)}</div>
                    <div class="trace-time">${time}</div>
                </div>
            </div>`;
        }).join('');
    }).catch(() => {});
}

function selectTrace(traceId) {
    selectedTraceId = traceId;
    document.querySelectorAll('.trace-item').forEach(el => {
        el.classList.toggle('selected', el.dataset.traceId === traceId);
    });
    loadTraceDetail(traceId);
}

// moved to top
// moved to top
// moved to top

function loadTraceDetail(traceId) {
    const detail = document.getElementById('trace-detail');
    if (!detail) return;

    fetch(`/api/traces/${traceId}`).then(r => r.json()).then(trace => {
        // Use demo data if empty and demo mode enabled
        if ((!trace?.spans?.length) && DemoData.enabled) {
            trace = DemoData.generateTraceDetail(traceId);
        }
        if (!trace?.spans?.length) {
            detail.innerHTML = '<div class="no-trace-selected"><div class="no-trace-selected-icon">🔍</div>No spans found</div>';
            return;
        }

        currentTraceData = trace;
        selectedSpanId = null;
        traceServiceFilter = null;
        renderWaterfall(detail, trace);
    }).catch(() => {
        if (DemoData.enabled) {
            const trace = DemoData.generateTraceDetail(traceId);
            currentTraceData = trace;
            selectedSpanId = null;
            traceServiceFilter = null;
            renderWaterfall(detail, trace);
        } else {
            detail.innerHTML = '<div class="no-trace-selected"><div class="no-trace-selected-icon">⚠️</div>Failed to load trace</div>';
        }
    });
}

function renderWaterfall(container, trace) {
    const spans = trace.spans;
    const minTime = Math.min(...spans.map(s => new Date(s.start_time).getTime()));
    const maxTime = Math.max(...spans.map(s => new Date(s.end_time).getTime()));
    const totalDuration = maxTime - minTime;

    // Build parent-child relationships
    const spanMap = {};
    spans.forEach(s => { spanMap[s.span_id] = s; s.children = []; s.depth = 0; });
    spans.forEach(s => {
        if (s.parent_span_id && spanMap[s.parent_span_id]) {
            spanMap[s.parent_span_id].children.push(s);
        }
    });

    // Calculate depths
    function setDepth(span, depth) {
        span.depth = depth;
        span.children.forEach(c => setDepth(c, depth + 1));
    }
    const rootSpans = spans.filter(s => !s.parent_span_id || !spanMap[s.parent_span_id]);
    rootSpans.forEach(s => setDepth(s, 0));

    // Sort by start time within each level
    spans.sort((a, b) => {
        if (a.depth !== b.depth) return a.depth - b.depth;
        return new Date(a.start_time) - new Date(b.start_time);
    });

    // Flatten tree maintaining parent-child order
    const orderedSpans = [];
    function addSpanAndChildren(span) {
        orderedSpans.push(span);
        span.children.sort((a, b) => new Date(a.start_time) - new Date(b.start_time));
        span.children.forEach(addSpanAndChildren);
    }
    rootSpans.sort((a, b) => new Date(a.start_time) - new Date(b.start_time));
    rootSpans.forEach(addSpanAndChildren);

    // Assign colors to services
    const serviceColors = {};
    const serviceSpanCounts = {};
    const colors = ['#3b82f6', '#10b981', '#8b5cf6', '#f59e0b', '#ef4444', '#ec4899', '#06b6d4', '#84cc16'];
    let colorIdx = 0;
    orderedSpans.forEach(s => {
        if (!serviceColors[s.service_name]) {
            serviceColors[s.service_name] = colors[colorIdx++ % colors.length];
            serviceSpanCounts[s.service_name] = 0;
        }
        serviceSpanCounts[s.service_name]++;
    });

    // Find critical path (longest path through the trace)
    const criticalPathIds = new Set();
    function findCriticalPath(span) {
        criticalPathIds.add(span.span_id);
        if (span.children.length === 0) return;
        const longestChild = span.children.reduce((a, b) =>
            (b.duration_ms || 0) > (a.duration_ms || 0) ? b : a
        );
        findCriticalPath(longestChild);
    }
    if (rootSpans.length > 0) {
        const longestRoot = rootSpans.reduce((a, b) =>
            (b.duration_ms || 0) > (a.duration_ms || 0) ? b : a
        );
        findCriticalPath(longestRoot);
    }

    // Stats
    const errorCount = spans.filter(s => s.status === 'ERROR').length;
    const avgDuration = spans.reduce((sum, s) => sum + (s.duration_ms || 0), 0) / spans.length;

    const timeMarkers = [0, 0.25, 0.5, 0.75, 1].map(p => formatTraceDuration(totalDuration * p));

    container.innerHTML = `
        <div class="waterfall-container" style="position:relative;">
            <div class="waterfall-toolbar">
                <button class="waterfall-btn" onclick="toggleCriticalPath()" id="critical-path-btn" title="Highlight Critical Path">Critical Path</button>
                <button class="waterfall-btn" onclick="expandAllSpans()" title="Expand All">Expand</button>
                <div class="waterfall-stats">
                    <div class="waterfall-stat">
                        <span class="waterfall-stat-value">${spans.length}</span>
                        <span>spans</span>
                    </div>
                    <div class="waterfall-stat">
                        <span class="waterfall-stat-value">${formatTraceDuration(totalDuration)}</span>
                        <span>total</span>
                    </div>
                    ${errorCount > 0 ? `
                    <div class="waterfall-stat">
                        <span class="waterfall-stat-value" style="color:#f4212e">${errorCount}</span>
                        <span>errors</span>
                    </div>` : ''}
                </div>
            </div>
            <div class="waterfall-legend" id="waterfall-legend">
                ${Object.entries(serviceColors).map(([svc, color]) => `
                    <div class="waterfall-legend-item" onclick="filterByService('${escapeHtml(svc)}')" data-service="${escapeHtml(svc)}">
                        <div class="waterfall-legend-color" style="background:${color}"></div>
                        <span class="waterfall-legend-name">${escapeHtml(svc)}</span>
                        <span class="waterfall-legend-count">${serviceSpanCounts[svc]}</span>
                    </div>
                `).join('')}
            </div>
            <div class="waterfall">
                <div class="waterfall-header">
                    <div class="waterfall-header-op">Operation</div>
                    <div class="waterfall-header-timeline">
                        ${timeMarkers.map(m => `<span>${m}</span>`).join('')}
                    </div>
                </div>
                <div class="waterfall-body" id="waterfall-body">
                    ${orderedSpans.map((span, idx) => renderSpanRow(span, minTime, totalDuration, serviceColors, criticalPathIds)).join('')}
                </div>
            </div>
            <div class="span-detail-panel" id="span-detail-panel">
                <div class="span-detail-header">
                    <div class="span-detail-title" id="span-detail-title">Span Details</div>
                    <button class="span-detail-close" onclick="closeSpanDetail()">×</button>
                </div>
                <div class="span-detail-body" id="span-detail-body"></div>
            </div>
        </div>
    `;

    // Store for interactions
    container.dataset.criticalPath = JSON.stringify([...criticalPathIds]);
}

function renderSpanRow(span, minTime, totalDuration, serviceColors, criticalPathIds) {
    const startOffset = new Date(span.start_time).getTime() - minTime;
    const left = totalDuration > 0 ? (startOffset / totalDuration * 100) : 0;
    const width = totalDuration > 0 ? ((span.duration_ms || 0) / totalDuration * 100) : 100;
    const color = serviceColors[span.service_name];
    const isError = span.status === 'ERROR';
    const isCritical = criticalPathIds.has(span.span_id);
    const kind = (span.kind || 'internal').toLowerCase();

    // Indent based on depth
    const indent = span.depth * 16;

    const kindIcons = { server: '◆', client: '◇', internal: '○', producer: '▷', consumer: '◁' };
    const kindIcon = kindIcons[kind] || '○';

    return `<div class="span-row ${isCritical ? 'critical-path' : ''} ${isError ? 'error' : ''}"
                data-span-id="${span.span_id}"
                data-service="${escapeHtml(span.service_name)}"
                onclick="selectSpan('${span.span_id}')">
        <div class="span-info" style="padding-left:${indent}px">
            <div class="span-icon" style="background:${color}20;color:${color}">${kindIcon}</div>
            <div class="span-details">
                <div class="span-name" title="${escapeHtml(span.name)}">${escapeHtml(span.name)}</div>
                <div class="span-meta">
                    <span class="span-service" style="color:${color}">${escapeHtml(span.service_name)}</span>
                    <span class="span-kind">${kind}</span>
                </div>
            </div>
        </div>
        <div class="span-timeline">
            <div class="timeline-grid">${'<div class="timeline-grid-line"></div>'.repeat(4)}</div>
            <div class="span-bar ${isError ? 'error' : ''}"
                 style="left:${left}%;width:${Math.max(width, 0.5)}%;background:linear-gradient(90deg,${color},${color}cc)">
                ${isError ? `<div class="span-status-icon" style="background:#f4212e"></div>` : ''}
            </div>
            <span class="span-duration">${formatTraceDuration(span.duration_ms)}</span>
        </div>
    </div>`;
}

function selectSpan(spanId) {
    selectedSpanId = spanId;

    // Update row selection
    document.querySelectorAll('.span-row').forEach(row => {
        row.classList.toggle('selected', row.dataset.spanId === spanId);
    });

    // Find span data
    const span = currentTraceData?.spans?.find(s => s.span_id === spanId);
    if (!span) return;

    // Open detail panel
    const panel = document.getElementById('span-detail-panel');
    const title = document.getElementById('span-detail-title');
    const body = document.getElementById('span-detail-body');

    if (!panel) return;

    title.textContent = span.name;

    const attributes = span.attributes || {};
    const attrEntries = Object.entries(attributes);

    body.innerHTML = `
        <div class="span-detail-section">
            <div class="span-detail-section-title">Timing</div>
            <div class="span-detail-grid">
                <div class="span-detail-stat">
                    <div class="span-detail-stat-value">${formatTraceDuration(span.duration_ms)}</div>
                    <div class="span-detail-stat-label">Duration</div>
                </div>
                <div class="span-detail-stat">
                    <div class="span-detail-stat-value">${new Date(span.start_time).toLocaleTimeString()}</div>
                    <div class="span-detail-stat-label">Start Time</div>
                </div>
            </div>
        </div>
        <div class="span-detail-section">
            <div class="span-detail-section-title">Info</div>
            <div class="span-detail-tags">
                <div class="span-detail-tag">
                    <span class="span-detail-tag-key">Service</span>
                    <span class="span-detail-tag-value">${escapeHtml(span.service_name)}</span>
                </div>
                <div class="span-detail-tag">
                    <span class="span-detail-tag-key">Kind</span>
                    <span class="span-detail-tag-value">${span.kind || 'internal'}</span>
                </div>
                <div class="span-detail-tag">
                    <span class="span-detail-tag-key">Status</span>
                    <span class="span-detail-tag-value" style="color:${span.status === 'ERROR' ? '#f4212e' : '#00ba7c'}">${span.status || 'OK'}</span>
                </div>
                <div class="span-detail-tag">
                    <span class="span-detail-tag-key">Span ID</span>
                    <span class="span-detail-tag-value">${span.span_id.slice(0, 16)}</span>
                </div>
                ${span.parent_span_id ? `
                <div class="span-detail-tag">
                    <span class="span-detail-tag-key">Parent</span>
                    <span class="span-detail-tag-value">${span.parent_span_id.slice(0, 16)}</span>
                </div>` : ''}
            </div>
        </div>
        ${attrEntries.length > 0 ? `
        <div class="span-detail-section">
            <div class="span-detail-section-title">Attributes (${attrEntries.length})</div>
            <div class="span-detail-tags">
                ${attrEntries.slice(0, 10).map(([k, v]) => `
                    <div class="span-detail-tag">
                        <span class="span-detail-tag-key">${escapeHtml(k)}</span>
                        <span class="span-detail-tag-value" title="${escapeHtml(String(v))}">${escapeHtml(String(v).slice(0, 50))}</span>
                    </div>
                `).join('')}
                ${attrEntries.length > 10 ? `<div style="color:#71767b;font-size:0.7rem;text-align:center;padding:8px;">+${attrEntries.length - 10} more</div>` : ''}
            </div>
        </div>` : ''}
    `;

    panel.classList.add('open');
}

function closeSpanDetail() {
    selectedSpanId = null;
    document.querySelectorAll('.span-row').forEach(row => row.classList.remove('selected'));
    document.getElementById('span-detail-panel')?.classList.remove('open');
}

function toggleCriticalPath() {
    const btn = document.getElementById('critical-path-btn');
    const isActive = btn?.classList.toggle('active');
    const container = document.getElementById('trace-detail');
    const criticalIds = JSON.parse(container?.dataset.criticalPath || '[]');

    document.querySelectorAll('.span-row').forEach(row => {
        if (isActive && !criticalIds.includes(row.dataset.spanId)) {
            row.classList.add('dimmed');
        } else {
            row.classList.remove('dimmed');
        }
    });
}

function filterByService(service) {
    const legendItems = document.querySelectorAll('.waterfall-legend-item');
    const rows = document.querySelectorAll('.span-row');

    if (traceServiceFilter === service) {
        // Clear filter
        traceServiceFilter = null;
        legendItems.forEach(item => item.classList.remove('dimmed'));
        rows.forEach(row => row.classList.remove('dimmed'));
    } else {
        // Apply filter
        traceServiceFilter = service;
        legendItems.forEach(item => {
            item.classList.toggle('dimmed', item.dataset.service !== service);
        });
        rows.forEach(row => {
            row.classList.toggle('dimmed', row.dataset.service !== service);
        });
    }
}

function expandAllSpans() {
    // Future: expand collapsed spans
}

function formatTraceDuration(ms) {
    if (ms === undefined || ms === null) return '-';
    if (ms < 1) return '<1ms';
    if (ms < 1000) return Math.round(ms) + 'ms';
    if (ms < 60000) return (ms / 1000).toFixed(2) + 's';
    return (ms / 60000).toFixed(1) + 'm';
}

// Flame graph - redesigned
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top
// moved to top

function updateFlameGraph() {
    fetch('/api/flamegraph').then(r => r.json()).then(data => {
        // Use demo data if empty and demo mode enabled
        if ((!data || !data.children || data.children.length === 0) && DemoData.enabled) {
            data = DemoData.generateFlameGraph();
        }
        flameData = data;
        flameZoomStack = [];
        renderFlameGraph();
    }).catch(() => {
        if (DemoData.enabled) {
            flameData = DemoData.generateFlameGraph();
            flameZoomStack = [];
            renderFlameGraph();
        } else {
            const el = document.getElementById('flamegraph');
            if (el) el.innerHTML = '<div class="flamegraph-empty"><div class="flamegraph-empty-icon">📊</div>Profiler unavailable</div>';
        }
    });
}

function clearFlameGraph() {
    fetch('/api/flamegraph/clear', { method: 'POST' }).then(() => {
        flameData = null;
        flameBaselineData = null;
        flameZoomStack = [];
        flameDiffMode = false;
        const el = document.getElementById('flamegraph');
        if (el) el.innerHTML = '<div class="flamegraph-empty"><div class="flamegraph-empty-icon">🔄</div>Cleared. Collecting new samples...</div>';
        document.getElementById('flamegraph-stats').innerHTML = '';
        document.getElementById('flamegraph-diff-btn')?.classList.remove('active');
    }).catch(() => {});
}

function searchFlameGraph(term) {
    flameSearchTerm = term.toLowerCase();
    renderFlameGraph();
}

function resetFlameGraphZoom() {
    flameZoomStack = [];
    renderFlameGraph();
}

function setFlameMode(mode) {
    flameMode = mode;
    document.getElementById('flamegraph-flame-btn')?.classList.toggle('active', mode === 'flame');
    document.getElementById('flamegraph-icicle-btn')?.classList.toggle('active', mode === 'icicle');
    renderFlameGraph();
}

function toggleDiffMode() {
    if (!flameDiffMode && flameData) {
        // Save current as baseline and enable diff mode
        flameBaselineData = JSON.parse(JSON.stringify(flameData));
        flameDiffMode = true;
        document.getElementById('flamegraph-diff-btn')?.classList.add('active');
        showToast('Baseline saved. Refresh to capture new profile for comparison.', 'info');
    } else {
        // Disable diff mode
        flameDiffMode = false;
        flameBaselineData = null;
        document.getElementById('flamegraph-diff-btn')?.classList.remove('active');
    }
    renderFlameGraph();
}

function zoomToFlameNode(index) {
    if (index < 0) {
        flameZoomStack = [];
    } else {
        flameZoomStack = flameZoomStack.slice(0, index + 1);
    }
    renderFlameGraph();
}

function getFrameColor(name, pct, selfPct) {
    // Determine category
    const isKernel = name.startsWith('0x') || name.includes('[unknown]') || name.includes('[kernel');
    const isRuntime = name.includes('runtime.') || name.includes('syscall') || name.includes('libc');
    const isGC = name.includes('gc') || name.includes('GC') || name.includes('malloc') || name.includes('free');

    if (isKernel) {
        return 'linear-gradient(135deg, #ef4444 0%, #dc2626 100%)'; // Red
    } else if (isGC) {
        return 'linear-gradient(135deg, #a855f7 0%, #9333ea 100%)'; // Purple
    } else if (isRuntime) {
        return 'linear-gradient(135deg, #6366f1 0%, #4f46e5 100%)'; // Indigo
    } else if (pct > 10) {
        return 'linear-gradient(135deg, #f97316 0%, #ea580c 100%)'; // Orange (hot)
    } else if (pct > 5) {
        return 'linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%)'; // Yellow (warm)
    } else if (pct > 2) {
        return 'linear-gradient(135deg, #22c55e 0%, #16a34a 100%)'; // Green
    } else {
        return 'linear-gradient(135deg, #3b82f6 0%, #2563eb 100%)'; // Blue (cold)
    }
}

function getFrameType(name) {
    if (name.startsWith('0x') || name.includes('[unknown]')) return 'kernel';
    if (name.includes('runtime.') || name.includes('syscall')) return 'runtime';
    if (name.includes('gc') || name.includes('GC')) return 'gc';
    if (name.includes('net/http') || name.includes('grpc')) return 'network';
    if (name.includes('database') || name.includes('sql')) return 'database';
    return 'user';
}

function renderFlameGraph() {
    const container = document.getElementById('flamegraph');
    const tooltip = document.getElementById('flamegraph-tooltip');
    const statsEl = document.getElementById('flamegraph-stats');
    const breadcrumbsEl = document.getElementById('flamegraph-breadcrumbs');

    if (!container) return;

    // Get data to render (zoomed or full)
    let data = flameZoomStack.length > 0 ? flameZoomStack[flameZoomStack.length - 1] : flameData;

    if (!data?.children?.length) {
        container.innerHTML = '<div class="flamegraph-empty"><div class="flamegraph-empty-icon">📊</div>Collecting samples... Click Refresh to update.</div>';
        if (statsEl) statsEl.innerHTML = '';
        if (breadcrumbsEl) breadcrumbsEl.style.display = 'none';
        return;
    }

    // Calculate total samples
    function countSamples(node) {
        return (node.value || 0) + (node.children || []).reduce((sum, c) => sum + countSamples(c), 0);
    }
    flameTotalSamples = countSamples(data);
    const rootTotalSamples = countSamples(flameData);

    // Update stats
    if (statsEl) {
        const hotFunctions = [];
        function findHot(node, depth = 0) {
            const total = countSamples(node);
            const pct = (total / rootTotalSamples) * 100;
            if (pct > 5 && depth > 0) hotFunctions.push({ name: node.name, pct });
            (node.children || []).forEach(c => findHot(c, depth + 1));
        }
        findHot(flameData);
        hotFunctions.sort((a, b) => b.pct - a.pct);

        statsEl.innerHTML = `
            <div class="flamegraph-stat">
                <span class="flamegraph-stat-value">${flameTotalSamples.toLocaleString()}</span>
                <span class="flamegraph-stat-label">samples</span>
            </div>
            ${hotFunctions.length > 0 ? `
            <div class="flamegraph-stat">
                <span class="flamegraph-stat-value" style="color:#f97316">${hotFunctions[0].pct.toFixed(1)}%</span>
                <span class="flamegraph-stat-label">hottest</span>
            </div>` : ''}
        `;
    }

    // Update breadcrumbs
    if (breadcrumbsEl) {
        if (flameZoomStack.length > 0) {
            breadcrumbsEl.style.display = 'flex';
            let html = `<span class="flamegraph-breadcrumb" onclick="zoomToFlameNode(-1)">root</span>`;
            flameZoomStack.forEach((node, i) => {
                const isLast = i === flameZoomStack.length - 1;
                html += `<span class="flamegraph-breadcrumb-sep">›</span>`;
                html += `<span class="flamegraph-breadcrumb ${isLast ? 'current' : ''}" ${isLast ? '' : `onclick="zoomToFlameNode(${i})"`}>${escapeHtml(node.name)}</span>`;
            });
            breadcrumbsEl.innerHTML = html;
        } else {
            breadcrumbsEl.style.display = 'none';
        }
    }

    container.innerHTML = '';
    const rect = container.getBoundingClientRect();
    const width = rect.width - 16;
    const frameHeight = 24;
    const padding = 8;
    const gap = 1;

    // Flatten the tree for rendering
    const frames = [];
    let maxDepth = 0;

    function processNode(node, depth, x, parentWidth) {
        if (!node || node.value === 0 && (!node.children || !node.children.length)) return;

        const nodeValue = node.value || 0;
        const childrenValue = (node.children || []).reduce((sum, c) => sum + (c.value || 0), 0);
        const totalValue = nodeValue + childrenValue;
        const nodeWidth = (totalValue / flameTotalSamples) * width;

        if (nodeWidth < 1) return;

        const matched = flameSearchTerm && node.name.toLowerCase().includes(flameSearchTerm);
        const faded = flameSearchTerm && !matched;

        frames.push({
            name: node.name,
            depth,
            x,
            width: nodeWidth,
            value: totalValue,
            self: nodeValue,
            matched,
            faded,
            node
        });

        maxDepth = Math.max(maxDepth, depth);

        // Process children
        let childX = x;
        (node.children || []).forEach(child => {
            const childTotal = (child.value || 0) + (child.children || []).reduce((sum, c) => sum + (c.value || 0), 0);
            const childWidth = (childTotal / flameTotalSamples) * width;
            processNode(child, depth + 1, childX, childWidth);
            childX += childWidth;
        });
    }

    processNode(data, 0, padding, width);

    // Render frames
    const totalHeight = (maxDepth + 1) * (frameHeight + gap) + padding * 2;
    container.style.height = Math.max(totalHeight, rect.height) + 'px';

    frames.forEach(f => {
        if (f.width < 2) return;

        const div = document.createElement('div');
        div.className = 'flamegraph-frame' + (f.matched ? ' matched' : '') + (f.faded ? ' faded' : '');

        // Position based on mode
        const y = flameMode === 'icicle'
            ? padding + f.depth * (frameHeight + gap)
            : totalHeight - padding - (f.depth + 1) * (frameHeight + gap);

        div.style.cssText = `left:${f.x}px;top:${y}px;width:${f.width - gap}px;height:${frameHeight}px;`;

        // Color based on function type and CPU time
        const pct = (f.value / rootTotalSamples) * 100;
        const selfPct = (f.self / rootTotalSamples) * 100;
        div.style.background = getFrameColor(f.name, pct, selfPct);

        // Label
        if (f.width > 50) {
            const label = document.createElement('span');
            label.className = 'flamegraph-frame-label';
            label.textContent = f.name;
            div.appendChild(label);
        }

        // Tooltip
        div.addEventListener('mouseenter', (e) => {
            const pctStr = pct.toFixed(2);
            const selfPctStr = selfPct.toFixed(2);
            const frameType = getFrameType(f.name);
            const color = getFrameColor(f.name, pct, selfPct);
            const isHot = pct > 10;
            const isWarm = pct > 5;

            tooltip.innerHTML = `
                <div class="flamegraph-tooltip-header">
                    <div class="flamegraph-tooltip-color" style="background:${color}"></div>
                    <div>
                        <div class="flamegraph-tooltip-name">${escapeHtml(f.name)}</div>
                        <span class="flamegraph-tooltip-type">${frameType}</span>
                    </div>
                </div>
                <div class="flamegraph-tooltip-grid">
                    <div class="flamegraph-tooltip-stat">
                        <div class="flamegraph-tooltip-stat-value ${isHot ? 'hot' : isWarm ? 'warm' : ''}">${pctStr}%</div>
                        <div class="flamegraph-tooltip-stat-label">Total Time</div>
                    </div>
                    <div class="flamegraph-tooltip-stat">
                        <div class="flamegraph-tooltip-stat-value">${selfPctStr}%</div>
                        <div class="flamegraph-tooltip-stat-label">Self Time</div>
                    </div>
                    <div class="flamegraph-tooltip-stat">
                        <div class="flamegraph-tooltip-stat-value">${f.value.toLocaleString()}</div>
                        <div class="flamegraph-tooltip-stat-label">Samples</div>
                    </div>
                    <div class="flamegraph-tooltip-stat">
                        <div class="flamegraph-tooltip-stat-value">${f.self.toLocaleString()}</div>
                        <div class="flamegraph-tooltip-stat-label">Self Samples</div>
                    </div>
                </div>
                <div class="flamegraph-tooltip-bar">
                    <div class="flamegraph-tooltip-bar-fill" style="width:${pct}%;background:${color}"></div>
                </div>
            `;
            tooltip.style.display = 'block';
            tooltip.style.left = Math.min(e.clientX + 15, window.innerWidth - 480) + 'px';
            tooltip.style.top = Math.min(e.clientY + 15, window.innerHeight - 250) + 'px';
        });

        div.addEventListener('mousemove', (e) => {
            tooltip.style.left = Math.min(e.clientX + 15, window.innerWidth - 480) + 'px';
            tooltip.style.top = Math.min(e.clientY + 15, window.innerHeight - 250) + 'px';
        });

        div.addEventListener('mouseleave', () => {
            tooltip.style.display = 'none';
        });

        // Click to zoom
        div.addEventListener('click', () => {
            if (f.node.children && f.node.children.length > 0) {
                flameZoomStack.push(f.node);
                renderFlameGraph();
            }
        });

        container.appendChild(div);
    });
}

// Watches
// moved to top

function loadWatchMetrics() {
    fetch('/api/watch/metrics').then(r => r.json()).then(data => {
        watchMetrics = data || [];
    }).catch(() => {});
}

function loadWatches() {
    fetch('/api/watches').then(r => r.json()).then(watches => {
        const list = document.getElementById('watches-list');
        if (!list) return;

        if (!watches?.length) {
            list.innerHTML = '<div class="empty-state">No watches configured. Click "+ New" to create one.</div>';
            return;
        }

        list.innerHTML = watches.map(w => {
            const stateClass = (w.state || 'no_data').toLowerCase();
            const metric = watchMetrics.find(m => m.id === w.metric) || { name: w.metric };
            return `<div class="watch-item">
                <div class="watch-info">
                    <div class="watch-name">${escapeHtml(w.name)}</div>
                    <div class="watch-condition">${metric.name} ${w.operator} ${w.threshold} for ${w.duration}</div>
                </div>
                <span class="watch-value">${w.last_value?.toFixed(1) || '-'}</span>
                <span class="watch-state ${stateClass}">${w.state || 'NO_DATA'}</span>
                <div class="watch-actions">
                    <button class="btn" onclick="deleteWatch('${w.id}')" title="Delete">X</button>
                </div>
            </div>`;
        }).join('');
    }).catch(() => {});
}

function showCreateWatch() {
    loadChannels();
    setTimeout(() => {
        const metricOptions = watchMetrics.map(m => `<option value="${m.id}">${m.name}</option>`).join('');
        const channelOptions = channels.length
            ? channels.map(c => `<label style="display:flex;align-items:center;gap:0.5rem;padding:0.3rem 0;"><input type="checkbox" class="watch-channel" value="${c.id}"> ${escapeHtml(c.name)}</label>`).join('')
            : '<div style="color:#71767b;font-size:0.75rem;">No channels configured. <a href="#" onclick="showChannels();return false;" style="color:#1d9bf0;">Add one</a></div>';
        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.innerHTML = `
            <div class="modal">
                <div class="modal-header">
                    <span class="modal-title">Create Watch</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group">
                        <label class="form-label">Name</label>
                        <input type="text" class="form-input" id="watch-name" placeholder="e.g., High CPU Alert">
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Metric</label>
                            <select class="form-select" id="watch-metric">${metricOptions}</select>
                        </div>
                        <div class="form-group">
                            <label class="form-label">Operator</label>
                            <select class="form-select" id="watch-operator">
                                <option value=">">Greater than (&gt;)</option>
                                <option value=">=">Greater or equal (&ge;)</option>
                                <option value="<">Less than (&lt;)</option>
                                <option value="<=">Less or equal (&le;)</option>
                            </select>
                        </div>
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Threshold</label>
                            <input type="number" class="form-input" id="watch-threshold" placeholder="80" step="any">
                        </div>
                        <div class="form-group">
                            <label class="form-label">Duration</label>
                            <select class="form-select" id="watch-duration">
                                <option value="0s">Instant</option>
                                <option value="1m">1 minute</option>
                                <option value="5m" selected>5 minutes</option>
                                <option value="15m">15 minutes</option>
                                <option value="30m">30 minutes</option>
                                <option value="1h">1 hour</option>
                            </select>
                        </div>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Notify via</label>
                        <div id="watch-channels">${channelOptions}</div>
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                    <button class="btn btn-primary" onclick="createWatch()">Create Watch</button>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
    }, 100);
}

function createWatch() {
    const name = document.getElementById('watch-name').value;
    const metric = document.getElementById('watch-metric').value;
    const operator = document.getElementById('watch-operator').value;
    const threshold = parseFloat(document.getElementById('watch-threshold').value);
    const duration = document.getElementById('watch-duration').value;
    const selectedChannels = Array.from(document.querySelectorAll('.watch-channel:checked')).map(cb => cb.value);

    if (!name || !metric || isNaN(threshold)) {
        alert('Please fill in all fields');
        return;
    }

    fetch('/api/watches', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, metric, operator, threshold, duration, enabled: true, channels: selectedChannels })
    }).then(r => {
        if (!r.ok) throw new Error('Failed to create watch');
        return r.json();
    }).then(() => {
        document.querySelector('.modal-overlay')?.remove();
        loadWatches();
    }).catch(err => alert(err.message));
}

function deleteWatch(id) {
    if (!confirm('Delete this watch?')) return;
    fetch(`/api/watches/${id}`, { method: 'DELETE' })
        .then(() => loadWatches())
        .catch(err => alert('Failed to delete: ' + err.message));
}

// Notification Channels
// moved to top

function loadChannels() {
    fetch('/api/watch/channels').then(r => r.json()).then(data => {
        channels = data || [];
    }).catch(() => { channels = []; });
}

function showChannels() {
    loadChannels();
    setTimeout(() => {
        const channelList = channels.length ? channels.map(c => `
            <div class="watch-item">
                <div class="watch-info">
                    <div class="watch-name">${escapeHtml(c.name)}</div>
                    <div class="watch-condition">${c.type} - ${c.type === 'webhook' ? JSON.parse(c.config).url : 'Slack'}</div>
                </div>
                <div class="watch-actions">
                    <button class="btn" onclick="testChannel('${c.id}')">Test</button>
                    <button class="btn" onclick="deleteChannel('${c.id}')">X</button>
                </div>
            </div>
        `).join('') : '<div class="empty-state">No channels configured</div>';

        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.innerHTML = `
            <div class="modal">
                <div class="modal-header">
                    <span class="modal-title">Notification Channels</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body" style="padding:0;">
                    ${channelList}
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="showCreateChannel()">+ Add Channel</button>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
    }, 100);
}

function showCreateChannel() {
    document.querySelector('.modal-overlay')?.remove();
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Add Notification Channel</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Name</label>
                    <input type="text" class="form-input" id="channel-name" placeholder="e.g., PagerDuty Prod">
                </div>
                <div class="form-group">
                    <label class="form-label">Type</label>
                    <select class="form-select" id="channel-type" onchange="updateChannelForm()">
                        <option value="webhook">Webhook (PagerDuty, Opsgenie, etc.)</option>
                        <option value="slack">Slack</option>
                    </select>
                </div>
                <div id="channel-config">
                    <div class="form-group">
                        <label class="form-label">Webhook URL</label>
                        <input type="url" class="form-input" id="channel-url" placeholder="https://events.pagerduty.com/v2/enqueue">
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="showChannels()">Back</button>
                <button class="btn btn-primary" onclick="createChannel()">Add Channel</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function updateChannelForm() {
    const type = document.getElementById('channel-type').value;
    const configDiv = document.getElementById('channel-config');
    if (type === 'slack') {
        configDiv.innerHTML = `
            <div class="form-group">
                <label class="form-label">Slack Webhook URL</label>
                <input type="url" class="form-input" id="channel-url" placeholder="https://hooks.slack.com/services/...">
            </div>
        `;
    } else {
        configDiv.innerHTML = `
            <div class="form-group">
                <label class="form-label">Webhook URL</label>
                <input type="url" class="form-input" id="channel-url" placeholder="https://events.pagerduty.com/v2/enqueue">
            </div>
        `;
    }
}

function createChannel() {
    const name = document.getElementById('channel-name').value;
    const type = document.getElementById('channel-type').value;
    const url = document.getElementById('channel-url').value;

    if (!name || !url) {
        alert('Please fill in all fields');
        return;
    }

    const config = type === 'slack'
        ? { webhook_url: url }
        : { url: url, method: 'POST' };

    fetch('/api/watch/channels', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, type, config })
    }).then(r => {
        if (!r.ok) throw new Error('Failed to create channel');
        return r.json();
    }).then(() => {
        loadChannels();
        showChannels();
    }).catch(err => alert(err.message));
}

function testChannel(id) {
    fetch(`/api/watch/channels/${id}/test`, { method: 'POST' })
        .then(r => {
            if (!r.ok) throw new Error('Test failed');
            alert('Test notification sent!');
        })
        .catch(err => alert('Test failed: ' + err.message));
}

function deleteChannel(id) {
    if (!confirm('Delete this channel?')) return;
    fetch(`/api/watch/channels/${id}`, { method: 'DELETE' })
        .then(() => showChannels())
        .catch(err => alert('Failed to delete: ' + err.message));
}

// Start data refresh timers
function startDataRefresh() {
    // Initial loads - staggered to prevent server overload
    setTimeout(initCharts, 100);
    updateSystemMetrics();
    updateStats();
    updateServiceMap();
    updateProcesses();

    // Core widgets - immediate
    loadTraceServices();
    loadTraces();
    loadWatchMetrics();
    loadWatches();

    // Secondary widgets - staggered
    setTimeout(updateFlameGraph, 200);
    setTimeout(loadLogServices, 250);
    setTimeout(searchLogs, 300);
    setTimeout(loadPatterns, 350);
    setTimeout(loadSynthetics, 400);
    setTimeout(loadSLOs, 450);
    setTimeout(loadContainers, 500);
    setTimeout(loadDeployments, 550);
    setTimeout(loadIncidents, 600);
    setTimeout(loadStatusPages, 650);
    setTimeout(loadCatalog, 700);
    setTimeout(loadCorrelations, 750);
    setTimeout(loadCluster, 800);
    setTimeout(loadAnomalies, 850);
    setTimeout(loadAlerts, 900);
    setTimeout(loadOnCallWidget, 950);
    setTimeout(loadAuditWidget, 1000);
    setTimeout(loadCostIntel, 1050);
    setTimeout(loadDBWatch, 1100);
    setTimeout(loadCardinality, 1150);
    setTimeout(loadNotifyWidget, 1200);

    // Set up WebSocket real-time updates (reduces polling overhead)
    setupWebSocketUpdates();

    // Periodic refreshes - WebSocket handles most real-time data
    // Polling is kept as fallback only (slower intervals since WebSocket is primary)
    setInterval(updateSystemMetrics, 15000);  // WebSocket handles real-time, poll for fallback
    setInterval(updateStats, 10000);
    setInterval(updateServiceMap, 60000);     // WebSocket handles real-time updates
    setInterval(updateProcesses, 5000);
    setInterval(loadTraceServices, 60000);
    setInterval(loadTraces, 30000);           // WebSocket streams new traces in real-time
    setInterval(loadWatches, 30000);          // WebSocket handles state changes
    setInterval(searchLogs, 30000);           // WebSocket streams logs in real-time
    setInterval(() => loadChartData(document.querySelector('.time-btn.active')?.dataset.dur || '15m'), 15000);

    // Resize handler
    window.addEventListener('resize', () => { renderServiceMap(); });
}

// Set up WebSocket subscriptions for real-time updates
function setupWebSocketUpdates() {
    if (typeof dwSocket === 'undefined') {
        console.log('[app] WebSocket client not loaded, using polling only');
        return;
    }

    // Subscribe to system stats for real-time CPU/memory updates
    dwSocket.onSystemStats((msg) => {
        if (msg.type === 'stats' && msg.payload) {
            // Update system metrics display
            const data = msg.payload;
            const cpuEl = document.getElementById('cpu-value');
            const memEl = document.getElementById('mem-value');
            const updateEl = document.getElementById('last-update');
            if (cpuEl) cpuEl.innerText = (data.cpu_percent || 0).toFixed(1) + '%';
            if (memEl) memEl.innerText = (data.memory_percent || 0).toFixed(1) + '%';
            if (updateEl) updateEl.innerText = 'Live';

            // Update charts if available
            if (cpuChart && memChart && data.cpu_percent !== undefined) {
                const now = new Date();
                cpuChart.data.labels.push(now);
                cpuChart.data.datasets[0].data.push(data.cpu_percent);
                if (cpuChart.data.labels.length > 60) {
                    cpuChart.data.labels.shift();
                    cpuChart.data.datasets[0].data.shift();
                }
                cpuChart.update('none');

                memChart.data.labels.push(now);
                memChart.data.datasets[0].data.push(data.memory_percent);
                if (memChart.data.labels.length > 60) {
                    memChart.data.labels.shift();
                    memChart.data.datasets[0].data.shift();
                }
                memChart.update('none');
            }
        }
    });

    // Subscribe to service map updates
    dwSocket.onServiceMapUpdate((msg) => {
        if (msg.type === 'update' && msg.payload) {
            svcMapData = msg.payload;
            renderServiceMap();
        }
    });

    // Subscribe to watch state changes for real-time alert updates
    dwSocket.onWatchStateChange((msg) => {
        if (msg.type === 'state_change' && msg.payload) {
            const { id, state, value } = msg.payload;
            // Update the specific watch in the UI
            const watchRow = document.querySelector(`[data-watch-id="${id}"]`);
            if (watchRow) {
                const stateEl = watchRow.querySelector('.watch-state');
                if (stateEl) {
                    stateEl.className = `watch-state state-${state.toLowerCase()}`;
                    stateEl.textContent = state;
                }
                const valueEl = watchRow.querySelector('.watch-value');
                if (valueEl) {
                    valueEl.textContent = value.toFixed(2);
                }
            }
            // Show toast for alerting state
            if (state === 'alerting') {
                showToast(`Watch alert: ${id}`, 'error');
            } else if (state === 'ok') {
                showToast(`Watch recovered: ${id}`, 'success');
            }
        }
    });

    // Subscribe to incident updates
    dwSocket.onIncidentUpdate((msg) => {
        if (msg.type === 'update') {
            loadIncidents(); // Refresh incidents list
        }
    });

    // Subscribe to anomaly detections
    dwSocket.onAnomalyDetected((msg) => {
        if (msg.type === 'detected' && msg.payload) {
            showToast('New anomaly detected', 'warning');
            loadAnomalies(); // Refresh anomalies
        }
    });

    // Subscribe to new traces for real-time trace streaming
    dwSocket.onNewTrace((msg) => {
        if (msg.type === 'new' && msg.payload) {
            // Update traces list if visible
            const tracesList = document.getElementById('traces-list');
            if (tracesList && tracesList.children.length > 0) {
                const trace = msg.payload;
                const row = document.createElement('div');
                row.className = 'trace-row';
                row.innerHTML = `
                    <span class="trace-id">${(trace.trace_id || '').substring(0, 8)}...</span>
                    <span class="trace-service">${trace.service_name || 'unknown'}</span>
                    <span class="trace-name">${trace.name || ''}</span>
                    <span class="trace-duration">${(trace.duration_ms || 0).toFixed(1)}ms</span>
                    <span class="trace-status ${trace.status === 'ERROR' ? 'status-error' : 'status-success'}">${trace.status || 'OK'}</span>
                `;
                tracesList.insertBefore(row, tracesList.firstChild);
                // Keep max 100 rows
                while (tracesList.children.length > 100) {
                    tracesList.removeChild(tracesList.lastChild);
                }
            }
        }
    });

    // Subscribe to log entries for real-time log streaming
    dwSocket.onLogEntry((msg) => {
        if (msg.type === 'entry' && msg.payload) {
            const logsList = document.getElementById('logs-list');
            if (logsList) {
                const entry = msg.payload;
                const levelClass = `log-level-${(entry.level || 'info').toLowerCase()}`;
                const row = document.createElement('div');
                row.className = `log-row ${levelClass}`;
                row.innerHTML = `
                    <span class="log-time">${new Date(entry.timestamp).toLocaleTimeString()}</span>
                    <span class="log-level">${(entry.level || 'INFO').toUpperCase()}</span>
                    <span class="log-service">${entry.service || '-'}</span>
                    <span class="log-message">${entry.message || ''}</span>
                `;
                logsList.insertBefore(row, logsList.firstChild);
                // Keep max 200 rows
                while (logsList.children.length > 200) {
                    logsList.removeChild(logsList.lastChild);
                }
            }
        }
    });

    // Subscribe to alert updates for real-time alerting
    dwSocket.onAlertUpdate((msg) => {
        if (msg.type === 'update' && msg.payload) {
            const alert = msg.payload;
            // Show toast for new alerts
            if (alert.state === 'firing') {
                const severity = alert.severity || 'warning';
                showToast(`Alert: ${alert.rule_name || 'Unknown'}`, severity === 'critical' ? 'error' : 'warning');
            } else if (alert.state === 'resolved') {
                showToast(`Resolved: ${alert.rule_name || 'Unknown'}`, 'success');
            }
            // Refresh alerts list
            loadAlerts();
        }
    });

    console.log('[app] WebSocket real-time updates enabled');
}

// Log functions
function loadLogServices() {
    fetch('/api/logs/services')
        .then(r => r.json())
        .then(services => {
            const select = document.getElementById('log-service');
            if (!select) return;
            const current = select.value;
            select.innerHTML = '<option value="">All Services</option>';
            (services || []).forEach(svc => {
                const opt = document.createElement('option');
                opt.value = svc;
                opt.textContent = svc;
                select.appendChild(opt);
            });
            select.value = current;
        })
        .catch(() => {});
}

function searchLogs() {
    const searchEl = document.getElementById('log-search');
    const levelEl = document.getElementById('log-level');
    const serviceEl = document.getElementById('log-service');
    const timeEl = document.getElementById('log-time');
    if (!searchEl) return;

    const params = new URLSearchParams();
    if (searchEl.value) params.set('q', searchEl.value);
    if (levelEl?.value) params.set('level', levelEl.value);
    if (serviceEl?.value) params.set('service', serviceEl.value);

    // Handle custom time range vs preset
    if (timeEl?.value === 'custom' && customLogTimeStart) {
        params.set('start', new Date(customLogTimeStart).toISOString());
        if (customLogTimeEnd) {
            params.set('end', new Date(customLogTimeEnd).toISOString());
        }
    } else if (timeEl?.value && timeEl.value !== 'custom') {
        params.set('since', timeEl.value);
    }
    params.set('limit', '100');

    // Update filter pills on search
    updateFilterPills();

    fetch('/api/logs?' + params.toString())
        .then(r => r.json())
        .then(result => {
            const list = document.getElementById('logs-list');
            if (!list) return;
            let entries = result?.data || result?.entries || [];
            // Use demo data if empty and demo mode enabled
            if (!entries.length && DemoData.enabled) {
                entries = DemoData.generateLogs().entries;
            }
            if (!entries.length) {
                list.innerHTML = '<div class="empty-state">No logs found</div>';
                return;
            }
            list.innerHTML = entries.map(log => {
                const time = new Date(log.timestamp).toLocaleTimeString();
                const traceLink = log.trace_id ?
                    `<span class="log-trace-link" onclick="selectTrace('${log.trace_id}')">${log.trace_id.slice(0,8)}</span>` : '';
                return `<div class="log-entry">
                    <span class="log-time">${time}</span>
                    <span class="log-level ${log.level}">${log.level}</span>
                    ${log.service ? `<span class="log-service">[${escapeHtml(log.service)}]</span>` : ''}
                    <span class="log-message">${escapeHtml(log.message)}</span>
                    ${traceLink}
                </div>`;
            }).join('');
        })
        .catch(() => {});
}

function refreshLogs() {
    loadLogServices();
    searchLogs();
}

// Live tail mode
// moved to top
// moved to top

function toggleLogTail() {
    logTailEnabled = !logTailEnabled;
    const btn = document.getElementById('log-tail-btn');
    const icon = document.getElementById('log-tail-icon');

    if (logTailEnabled) {
        btn.style.background = '#1a3d2e';
        btn.style.borderColor = '#00ba7c';
        btn.style.color = '#00ba7c';
        icon.textContent = '⏸';
        // Poll every 2 seconds
        logTailInterval = setInterval(() => {
            searchLogs();
            // Auto-scroll to bottom
            const list = document.getElementById('logs-list');
            if (list) list.scrollTop = 0;
        }, 2000);
        showToast('Live tail enabled', 'success');
    } else {
        btn.style.background = '';
        btn.style.borderColor = '';
        btn.style.color = '';
        icon.textContent = '▶';
        if (logTailInterval) {
            clearInterval(logTailInterval);
            logTailInterval = null;
        }
        showToast('Live tail disabled', 'info');
    }
}

// Filter pills and custom time range
// moved to top
// moved to top

function updateLogFilters() {
    const timeEl = document.getElementById('log-time');
    if (timeEl?.value === 'custom') {
        showCustomTimeRangeModal();
        return;
    }
    updateFilterPills();
    searchLogs();
}

function updateFilterPills() {
    const pillsEl = document.getElementById('log-filter-pills');
    if (!pillsEl) return;

    const level = document.getElementById('log-level')?.value;
    const service = document.getElementById('log-service')?.value;
    const search = document.getElementById('log-search')?.value;
    const time = document.getElementById('log-time')?.value;

    const pills = [];

    if (search) {
        pills.push(`<span class="filter-pill">query: "${escapeHtml(search)}" <span onclick="clearLogFilter('search')">&times;</span></span>`);
    }
    if (level) {
        pills.push(`<span class="filter-pill level-${level}">level: ${level} <span onclick="clearLogFilter('level')">&times;</span></span>`);
    }
    if (service) {
        pills.push(`<span class="filter-pill">service: ${escapeHtml(service)} <span onclick="clearLogFilter('service')">&times;</span></span>`);
    }
    if (time === 'custom' && customLogTimeStart) {
        const start = new Date(customLogTimeStart).toLocaleString();
        const end = customLogTimeEnd ? new Date(customLogTimeEnd).toLocaleString() : 'now';
        pills.push(`<span class="filter-pill">time: ${start} - ${end} <span onclick="clearLogFilter('time')">&times;</span></span>`);
    }

    if (pills.length > 0) {
        pillsEl.innerHTML = pills.join('') + `<span class="filter-clear" onclick="clearAllLogFilters()">Clear all</span>`;
        pillsEl.style.display = 'flex';
    } else {
        pillsEl.style.display = 'none';
    }
}

function clearLogFilter(type) {
    switch(type) {
        case 'search':
            document.getElementById('log-search').value = '';
            break;
        case 'level':
            document.getElementById('log-level').value = '';
            break;
        case 'service':
            document.getElementById('log-service').value = '';
            break;
        case 'time':
            document.getElementById('log-time').value = '1h';
            customLogTimeStart = null;
            customLogTimeEnd = null;
            break;
    }
    updateFilterPills();
    searchLogs();
}

function clearAllLogFilters() {
    document.getElementById('log-search').value = '';
    document.getElementById('log-level').value = '';
    document.getElementById('log-service').value = '';
    document.getElementById('log-time').value = '1h';
    customLogTimeStart = null;
    customLogTimeEnd = null;
    updateFilterPills();
    searchLogs();
}

function showCustomTimeRangeModal() {
    const now = new Date();
    const hourAgo = new Date(now.getTime() - 60 * 60 * 1000);

    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => {
        if (e.target === modal) {
            modal.remove();
            document.getElementById('log-time').value = '1h';
        }
    };
    modal.innerHTML = `
        <div class="modal" style="max-width:400px">
            <div class="modal-header">
                <span class="modal-title">Custom Time Range</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove();document.getElementById('log-time').value='1h';">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Start Time</label>
                    <input type="datetime-local" id="custom-time-start" value="${hourAgo.toISOString().slice(0,16)}">
                </div>
                <div class="form-group">
                    <label>End Time</label>
                    <input type="datetime-local" id="custom-time-end" value="${now.toISOString().slice(0,16)}">
                </div>
                <div style="display:flex;gap:0.5rem;margin-top:1rem;">
                    <button class="btn" onclick="setQuickTimeRange(15)">15m</button>
                    <button class="btn" onclick="setQuickTimeRange(60)">1h</button>
                    <button class="btn" onclick="setQuickTimeRange(360)">6h</button>
                    <button class="btn" onclick="setQuickTimeRange(1440)">24h</button>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove();document.getElementById('log-time').value='1h';">Cancel</button>
                <button class="btn btn-primary" onclick="applyCustomTimeRange()">Apply</button>
            </div>
        </div>`;
    document.body.appendChild(modal);
}

function setQuickTimeRange(minutes) {
    const now = new Date();
    const start = new Date(now.getTime() - minutes * 60 * 1000);
    document.getElementById('custom-time-start').value = start.toISOString().slice(0,16);
    document.getElementById('custom-time-end').value = now.toISOString().slice(0,16);
}

function applyCustomTimeRange() {
    customLogTimeStart = document.getElementById('custom-time-start').value;
    customLogTimeEnd = document.getElementById('custom-time-end').value;
    document.querySelector('.modal-overlay')?.remove();
    updateFilterPills();
    searchLogs();
}

// Synthetics functions
function loadSynthetics() {
    fetch('/api/synthetics/checks')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(result => {
            const list = document.getElementById('synthetics-list');
            if (!list) return;

            // Handle wrapped response or null
            let checks = result?.data || result || [];
            if (!Array.isArray(checks)) checks = [];

            // Use demo data if empty and demo mode enabled
            if (!checks.length && DemoData.enabled) {
                checks = DemoData.generateSynthetics();
            }
            if (!checks.length) {
                list.innerHTML = '<div class="empty-state">No synthetic checks configured. Click "+ New" to create one.</div>';
                return;
            }

            syntheticChecks = checks;
            renderSyntheticsList(list, checks);
        })
        .catch(() => {
            if (DemoData.enabled) {
                const list = document.getElementById('synthetics-list');
                if (list) renderSyntheticsList(list, DemoData.generateSynthetics());
            }
        });
}

function renderSyntheticsList(list, checks) {
    list.innerHTML = checks.map(c => {
        const statusClass = (c.status || 'unknown').toLowerCase();
        const latency = c.last_latency_ms ? formatLatency(c.last_latency_ms) : '-';
        return `<div class="synth-item">
            <div class="synth-info">
                <div class="synth-name">${escapeHtml(c.name)}</div>
                <div class="synth-url">${escapeHtml(c.url)}</div>
            </div>
            <span class="synth-latency">${latency}</span>
            <span class="synth-status ${statusClass}">${c.status || 'unknown'}</span>
            <div class="synth-actions">
                <button class="btn" onclick="runSynthetic('${c.id}')" title="Run Now">Run</button>
                <button class="btn" onclick="deleteSynthetic('${c.id}')" title="Delete">X</button>
            </div>
        </div>`;
    }).join('');
}

function showCreateSynthetic() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Synthetic Check</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Name</label>
                    <input type="text" class="form-input" id="synth-name" placeholder="e.g., Homepage Check">
                </div>
                <div class="form-group">
                    <label class="form-label">URL</label>
                    <input type="url" class="form-input" id="synth-url" placeholder="https://example.com/health">
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Method</label>
                        <select class="form-select" id="synth-method">
                            <option value="GET">GET</option>
                            <option value="POST">POST</option>
                            <option value="PUT">PUT</option>
                            <option value="HEAD">HEAD</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Interval (seconds)</label>
                        <input type="number" class="form-input" id="synth-interval" value="60" min="10" max="3600">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Timeout (seconds)</label>
                        <input type="number" class="form-input" id="synth-timeout" value="10" min="1" max="60">
                    </div>
                    <div class="form-group">
                        <label class="form-label">Expected Status</label>
                        <input type="number" class="form-input" id="synth-status" value="200" placeholder="200">
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createSynthetic()">Create Check</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createSynthetic() {
    const name = document.getElementById('synth-name').value;
    const url = document.getElementById('synth-url').value;
    const method = document.getElementById('synth-method').value;
    const interval = parseInt(document.getElementById('synth-interval').value) || 60;
    const timeout = parseInt(document.getElementById('synth-timeout').value) || 10;
    const expectedStatus = document.getElementById('synth-status').value;

    if (!name || !url) {
        alert('Please fill in name and URL');
        return;
    }

    const assertions = [];
    if (expectedStatus) {
        assertions.push({
            type: 'status_code',
            operator: 'equals',
            value: expectedStatus
        });
    }

    fetch('/api/synthetics/checks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            name, url, method, interval, timeout,
            type: 'http',
            enabled: true,
            assertions
        })
    }).then(r => {
        if (!r.ok) throw new Error('Failed to create check');
        return r.json();
    }).then(() => {
        document.querySelector('.modal-overlay')?.remove();
        loadSynthetics();
    }).catch(err => alert(err.message));
}

function runSynthetic(id) {
    fetch(`/api/synthetics/checks/${id}/run`, { method: 'POST' })
        .then(r => r.json())
        .then(result => {
            if (result.status === 'up') {
                alert(`Check passed! Latency: ${result.latency_ms}ms`);
            } else {
                alert(`Check failed: ${result.error || 'Unknown error'}`);
            }
            loadSynthetics();
        })
        .catch(err => alert('Failed to run check: ' + err.message));
}

function deleteSynthetic(id) {
    if (!confirm('Delete this synthetic check?')) return;
    fetch(`/api/synthetics/checks/${id}`, { method: 'DELETE' })
        .then(() => loadSynthetics())
        .catch(err => alert('Failed to delete: ' + err.message));
}

// Initialize synthetics
setTimeout(loadSynthetics, 500);
setInterval(loadSynthetics, 10000);

// SLO functions
// moved to top

function loadSyntheticChecksForSLO() {
    return fetch('/api/synthetics/checks')
        .then(r => r.json())
        .then(checks => {
            syntheticChecks = checks || [];
            return checks;
        })
        .catch(() => { syntheticChecks = []; return []; });
}

function loadSLOs() {
    fetch('/api/slos')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(slos => {
            const list = document.getElementById('slos-list');
            if (!list) return;

            // Handle wrapped response
            if (slos?.data) slos = slos.data;

            // Use demo data if empty and demo mode enabled
            if (!slos?.length && DemoData.enabled) {
                slos = DemoData.generateSLOs();
            }
            if (!slos?.length) {
                list.innerHTML = '<div class="empty-state">No SLOs configured. Click "+ New" to create one.</div>';
                return;
            }

            // Render SLOs - handle {slo, state} wrapper
            list.innerHTML = slos.map(item => {
                const slo = item.slo || item;
                const state = item.state || {};
                const statusClass = (state.status || 'unknown').toLowerCase();
                const target = slo.target?.toFixed(1) || '-';
                const current = state.current_value?.toFixed(2) || '-';
                return `<div class="slo-item">
                    <div class="slo-header">
                        <span class="slo-name">${escapeHtml(slo.name)}</span>
                        <span class="slo-status ${statusClass}">${state.status || 'NO DATA'}</span>
                    </div>
                    <div class="slo-metrics">
                        <div class="slo-metric"><div class="slo-metric-value">${current}%</div><div class="slo-metric-label">Current</div></div>
                        <div class="slo-metric"><div class="slo-metric-value">${target}%</div><div class="slo-metric-label">Target</div></div>
                        <div class="slo-metric"><div class="slo-metric-value">${slo.window || '-'}</div><div class="slo-metric-label">Window</div></div>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            if (DemoData.enabled) {
                const list = document.getElementById('slos-list');
                if (!list) return;
                const slos = DemoData.generateSLOs();
                list.innerHTML = slos.map(item => {
                    const slo = item.slo;
                    const state = item.state || {};
                    const statusClass = (state.status || 'no_data').toLowerCase().replace(' ', '_');
                    const currentValue = state.current_value?.toFixed(2) || '-';
                    const target = slo.target?.toFixed(1) || '-';
                    const budgetUsed = state.budget_used_pct || 0;
                    const budgetRemaining = state.budget_remaining?.toFixed(1) || '-';
                    const valueClass = state.current_value >= slo.target ? 'good' : state.current_value >= slo.target - 1 ? 'warn' : 'bad';
                    const budgetClass = budgetUsed < 50 ? 'healthy' : budgetUsed < 80 ? 'warning' : 'critical';
                    return `<div class="slo-item">
                        <div class="slo-header">
                            <span class="slo-name">${escapeHtml(slo.name)}</span>
                            <span class="slo-status ${statusClass}">${state.status || 'NO DATA'}</span>
                        </div>
                        <div class="slo-metrics">
                            <div class="slo-metric"><div class="slo-metric-value ${valueClass}">${currentValue}%</div><div class="slo-metric-label">Current</div></div>
                            <div class="slo-metric"><div class="slo-metric-value">${target}%</div><div class="slo-metric-label">Target</div></div>
                            <div class="slo-metric"><div class="slo-metric-value">${slo.window}</div><div class="slo-metric-label">Window</div></div>
                        </div>
                        <div class="slo-budget">
                            <div class="slo-budget-bar"><div class="slo-budget-fill ${budgetClass}" style="width:${Math.min(budgetUsed, 100)}%"></div></div>
                            <div class="slo-budget-text"><span>Error Budget: ${budgetRemaining} min remaining</span><span>${budgetUsed.toFixed(1)}% used</span></div>
                        </div>
                    </div>`;
                }).join('');
            }
        });
}

function showCreateSLO() {
    loadSyntheticChecksForSLO().then(() => {
        const checkOptions = syntheticChecks.length
            ? syntheticChecks.map(c => `<option value="${c.id}">${escapeHtml(c.name)}</option>`).join('')
            : '<option value="">No synthetic checks available</option>';

        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.innerHTML = `
            <div class="modal">
                <div class="modal-header">
                    <span class="modal-title">Create SLO</span>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                </div>
                <div class="modal-body">
                    <div class="form-group">
                        <label class="form-label">Name</label>
                        <input type="text" class="form-input" id="slo-name" placeholder="e.g., API Availability">
                    </div>
                    <div class="form-group">
                        <label class="form-label">Description</label>
                        <input type="text" class="form-input" id="slo-desc" placeholder="e.g., Main API must be available 99.9%">
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Type</label>
                            <select class="form-select" id="slo-type">
                                <option value="availability">Availability (%)</option>
                                <option value="latency">Latency (P95)</option>
                                <option value="error_rate">Error Rate</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label class="form-label">Target (%)</label>
                            <input type="number" class="form-input" id="slo-target" value="99.9" step="0.1" min="0" max="100">
                        </div>
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label class="form-label">Time Window</label>
                            <select class="form-select" id="slo-window">
                                <option value="7d">7 Days</option>
                                <option value="30d" selected>30 Days</option>
                                <option value="90d">90 Days</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label class="form-label">Latency Threshold (ms)</label>
                            <input type="number" class="form-input" id="slo-threshold" value="200" placeholder="For latency SLOs">
                        </div>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Data Source (Synthetic Check)</label>
                        <select class="form-select" id="slo-source">${checkOptions}</select>
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                    <button class="btn btn-primary" onclick="createSLO()">Create SLO</button>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
    });
}

function createSLO() {
    const name = document.getElementById('slo-name').value;
    const description = document.getElementById('slo-desc').value;
    const type = document.getElementById('slo-type').value;
    const target = parseFloat(document.getElementById('slo-target').value);
    const window = document.getElementById('slo-window').value;
    const threshold = parseFloat(document.getElementById('slo-threshold').value) || 0;
    const sourceId = document.getElementById('slo-source').value;

    if (!name || !sourceId) {
        alert('Please fill in name and select a data source');
        return;
    }

    if (isNaN(target) || target <= 0 || target > 100) {
        alert('Target must be between 0 and 100');
        return;
    }

    fetch('/api/slos', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            name,
            description,
            type,
            target,
            window,
            threshold,
            enabled: true,
            source: {
                type: 'synthetics',
                id: sourceId
            }
        })
    }).then(r => {
        if (!r.ok) throw new Error('Failed to create SLO');
        return r.json();
    }).then(() => {
        document.querySelector('.modal-overlay')?.remove();
        loadSLOs();
    }).catch(err => alert(err.message));
}

function deleteSLO(id) {
    if (!confirm('Delete this SLO?')) return;
    fetch(`/api/slos/${id}`, { method: 'DELETE' })
        .then(() => loadSLOs())
        .catch(err => alert('Failed to delete: ' + err.message));
}

// Log Patterns functions
function loadPatterns() {
    const filter = document.getElementById('pattern-filter')?.value || 'all';
    fetch(`/api/logs/patterns?filter=${filter}&limit=20`)
        .then(r => r.json())
        .then(data => {
            // Use demo data if empty and demo mode enabled
            if (DemoData.enabled && (!data?.patterns || data.patterns.length === 0)) {
                data = DemoData.generatePatterns();
            }
            const stats = data.stats || {};
            const patterns = data.patterns || [];

            // Update stats
            const statsEl = document.getElementById('pattern-stats');
            if (statsEl) {
                statsEl.innerHTML = `
                    <div class="pattern-stat">
                        <span class="pattern-stat-value">${stats.total_patterns || 0}</span>
                        <span class="pattern-stat-label">Patterns</span>
                    </div>
                    <div class="pattern-stat">
                        <span class="pattern-stat-value">${stats.total_matches || 0}</span>
                        <span class="pattern-stat-label">Total Logs</span>
                    </div>
                    <div class="pattern-stat">
                        <span class="pattern-stat-value">${stats.matches_last_hour || 0}</span>
                        <span class="pattern-stat-label">Last Hour</span>
                    </div>
                    <div class="pattern-stat">
                        <span class="pattern-stat-value">${stats.new_patterns_today || 0}</span>
                        <span class="pattern-stat-label">New Today</span>
                    </div>
                `;
            }

            // Update patterns list
            const list = document.getElementById('patterns-list');
            if (!list) return;

            if (!patterns?.length) {
                list.innerHTML = '<div class="empty-state">No patterns detected yet. Send some logs to see patterns emerge.</div>';
                return;
            }

            list.innerHTML = patterns.map(p => {
                const trend = p.trend || 'stable';
                const trendLabel = trend === 'increasing' ? 'Increasing' :
                                  trend === 'decreasing' ? 'Decreasing' :
                                  trend === 'new' ? 'New' : 'Stable';
                const examples = (p.examples || []).slice(0, 2);
                const lastSeen = p.last_seen ? new Date(p.last_seen).toLocaleString() : '-';

                return `<div class="pattern-item">
                    <div class="pattern-header">
                        <span class="pattern-trend ${trend}">${trendLabel}</span>
                        <span class="pattern-count">${p.count.toLocaleString()} occurrences</span>
                        ${p.count_last_hr ? `<span style="color:#71767b;font-size:0.65rem;">(${p.count_last_hr} last hour)</span>` : ''}
                    </div>
                    <div class="pattern-signature" title="${escapeHtml(p.signature)}">${escapeHtml(p.signature)}</div>
                    <div class="pattern-meta">
                        <span>Level: ${p.level || '-'}</span>
                        <span>Service: ${p.service || '-'}</span>
                        <span>Last seen: ${lastSeen}</span>
                    </div>
                    ${examples.length ? `<div class="pattern-examples">
                        <span style="color:#71767b">Examples:</span>
                        ${examples.map(ex => `<div class="pattern-example" title="${escapeHtml(ex)}">${escapeHtml(ex)}</div>`).join('')}
                    </div>` : ''}
                </div>`;
            }).join('');
        })
        .catch(() => {});
}

// Container functions
function loadContainers() {
    fetch('/api/containers')
        .then(r => r.json())
        .then(data => {
            if (!Array.isArray(data)) data = [];

            // Update summary
            const running = data.filter(d => d.container?.state === 'running').length;
            const stopped = data.filter(d => d.container?.state !== 'running').length;
            let totalCpu = 0, totalMem = 0;
            data.forEach(d => {
                if (d.stats) {
                    totalCpu += d.stats.cpu_percent || 0;
                    totalMem += d.stats.memory_usage || 0;
                }
            });

            const summaryEl = document.getElementById('container-summary');
            if (summaryEl) {
                summaryEl.innerHTML = `
                    <div class="container-stat">
                        <span class="container-stat-value">${data.length}</span>
                        <span class="container-stat-label">Total</span>
                    </div>
                    <div class="container-stat">
                        <span class="container-stat-value" style="color:#00ba7c">${running}</span>
                        <span class="container-stat-label">Running</span>
                    </div>
                    <div class="container-stat">
                        <span class="container-stat-value" style="color:#71767b">${stopped}</span>
                        <span class="container-stat-label">Stopped</span>
                    </div>
                    <div class="container-stat">
                        <span class="container-stat-value">${totalCpu.toFixed(1)}%</span>
                        <span class="container-stat-label">CPU</span>
                    </div>
                    <div class="container-stat">
                        <span class="container-stat-value">${formatBytes(totalMem)}</span>
                        <span class="container-stat-label">Memory</span>
                    </div>
                `;
            }

            // Update list
            const list = document.getElementById('containers-list');
            if (!list) return;

            if (!data.length) {
                list.innerHTML = '<div class="empty-state">No containers found. Is Docker running?</div>';
                return;
            }

            list.innerHTML = data.map(d => {
                const c = d.container || {};
                const s = d.stats || {};
                const state = c.state || 'unknown';
                const cpuPct = (s.cpu_percent || 0).toFixed(1);
                const memPct = (s.memory_percent || 0).toFixed(1);
                const memUsed = formatBytes(s.memory_usage || 0);
                const netRx = formatBytes(s.network_rx_bytes || 0);
                const netTx = formatBytes(s.network_tx_bytes || 0);

                return `<div class="container-item">
                    <div class="container-header">
                        <span class="container-state ${state}">${state}</span>
                        <span class="container-name">${escapeHtml(c.name || c.id)}</span>
                    </div>
                    <div class="container-image">${escapeHtml(c.image || '-')}</div>
                    ${state === 'running' ? `
                    <div class="container-metrics">
                        <div class="container-metric">
                            <span class="container-metric-value">${cpuPct}%</span>
                            <span class="container-metric-label">CPU</span>
                            <div class="container-bar"><div class="container-bar-fill cpu" style="width:${Math.min(cpuPct, 100)}%"></div></div>
                        </div>
                        <div class="container-metric">
                            <span class="container-metric-value">${memUsed}</span>
                            <span class="container-metric-label">Memory (${memPct}%)</span>
                            <div class="container-bar"><div class="container-bar-fill mem" style="width:${Math.min(memPct, 100)}%"></div></div>
                        </div>
                        <div class="container-metric">
                            <span class="container-metric-value">${netRx} / ${netTx}</span>
                            <span class="container-metric-label">Net RX / TX</span>
                        </div>
                        <div class="container-metric">
                            <span class="container-metric-value">${s.pids || 0}</span>
                            <span class="container-metric-label">PIDs</span>
                        </div>
                    </div>
                    ` : `<div style="font-size:0.7rem;color:#71767b;margin-top:0.3rem;">${escapeHtml(c.status || '')}</div>`}
                </div>`;
            }).join('');
        })
        .catch(() => {
            const list = document.getElementById('containers-list');
            if (list) list.innerHTML = '<div class="empty-state">Container monitoring not available</div>';
        });
}

// Deployment functions
function loadDeployments() {
    fetch('/api/deploys')
        .then(r => r.json())
        .then(data => {
            if (!Array.isArray(data)) data = [];
            // Use demo data if empty and demo mode enabled
            if (!data.length && DemoData.enabled) {
                data = DemoData.generateDeployments();
            }

            // Load stats
            fetch('/api/deploys/stats')
                .then(r => r.json())
                .then(stats => {
                    // Use demo stats if empty
                    if (DemoData.enabled && !stats?.total_deployments) {
                        stats = DemoData.generateDeployStats();
                    }
                    const summaryEl = document.getElementById('deploy-summary');
                    if (summaryEl) {
                        summaryEl.innerHTML = `
                            <div class="deploy-stat">
                                <span class="deploy-stat-value">${stats.total_deployments || 0}</span>
                                <span class="deploy-stat-label">Total</span>
                            </div>
                            <div class="deploy-stat">
                                <span class="deploy-stat-value">${stats.deploys_today || 0}</span>
                                <span class="deploy-stat-label">Today</span>
                            </div>
                            <div class="deploy-stat">
                                <span class="deploy-stat-value">${stats.deploys_this_week || 0}</span>
                                <span class="deploy-stat-label">This Week</span>
                            </div>
                            <div class="deploy-stat">
                                <span class="deploy-stat-value" style="color:#00ba7c">${(stats.success_rate || 0).toFixed(0)}%</span>
                                <span class="deploy-stat-label">Success Rate</span>
                            </div>
                            <div class="deploy-stat">
                                <span class="deploy-stat-value" style="color:${stats.recent_failures > 0 ? '#f4212e' : '#71767b'}">${stats.recent_failures || 0}</span>
                                <span class="deploy-stat-label">Recent Failures</span>
                            </div>
                        `;
                    }
                })
                .catch(() => {
                    if (DemoData.enabled) {
                        const stats = DemoData.generateDeployStats();
                        const summaryEl = document.getElementById('deploy-summary');
                        if (summaryEl) {
                            summaryEl.innerHTML = `
                                <div class="deploy-stat"><span class="deploy-stat-value">${stats.total_deployments}</span><span class="deploy-stat-label">Total</span></div>
                                <div class="deploy-stat"><span class="deploy-stat-value">${stats.deploys_today}</span><span class="deploy-stat-label">Today</span></div>
                                <div class="deploy-stat"><span class="deploy-stat-value">${stats.deploys_this_week}</span><span class="deploy-stat-label">This Week</span></div>
                                <div class="deploy-stat"><span class="deploy-stat-value" style="color:#00ba7c">${stats.success_rate.toFixed(0)}%</span><span class="deploy-stat-label">Success Rate</span></div>
                                <div class="deploy-stat"><span class="deploy-stat-value" style="color:${stats.recent_failures > 0 ? '#f4212e' : '#71767b'}">${stats.recent_failures}</span><span class="deploy-stat-label">Recent Failures</span></div>
                            `;
                        }
                    }
                });

            // Update list
            const list = document.getElementById('deployments-list');
            if (!list) return;

            if (!data.length) {
                list.innerHTML = '<div class="empty-state">No deployments recorded. Click "+ Deploy" to record one.</div>';
                return;
            }

            list.innerHTML = data.slice(0, 20).map(d => {
                const time = new Date(d.timestamp).toLocaleString();
                const relTime = timeAgo(new Date(d.timestamp));
                const commitLink = d.commit_url ?
                    `<a href="${escapeHtml(d.commit_url)}" target="_blank" class="deploy-commit">${escapeHtml(d.commit_sha?.substring(0, 7) || '')}</a>` :
                    (d.commit_sha ? `<span class="deploy-commit">${escapeHtml(d.commit_sha.substring(0, 7))}</span>` : '');

                return `<div class="deploy-item" onclick="showDeployImpact('${d.id}')" style="cursor:pointer">
                    <div class="deploy-marker ${d.status || 'success'}"></div>
                    <div class="deploy-info">
                        <div class="deploy-header">
                            <span class="deploy-service">${escapeHtml(d.service)}</span>
                            <span class="deploy-version">${escapeHtml(d.version)}</span>
                            <span class="deploy-env ${d.environment || 'prod'}">${d.environment || 'prod'}</span>
                        </div>
                        <div class="deploy-meta">
                            <span class="deploy-time" title="${time}">${relTime}</span>
                            ${d.user ? `<span class="deploy-user">by ${escapeHtml(d.user)}</span>` : ''}
                            ${commitLink}
                            ${d.duration_ms ? `<span>${d.duration_ms}ms</span>` : ''}
                        </div>
                    </div>
                    <div class="deploy-actions" onclick="event.stopPropagation()">
                        <button class="btn" onclick="showDeployImpact('${d.id}')" title="View impact">&#128200;</button>
                        ${d.status === 'success' ? `<button class="btn" onclick="markDeployFailed('${d.id}')" title="Mark as failed">&#10006;</button>` : ''}
                        ${d.status === 'failed' ? `<button class="btn" onclick="markDeployRolledBack('${d.id}')" title="Mark as rolled back">&#8634;</button>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            const list = document.getElementById('deployments-list');
            if (list) list.innerHTML = '<div class="empty-state">Deployments not available</div>';
        });
}

function timeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    if (seconds < 60) return 'just now';
    if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
    if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
    return Math.floor(seconds / 86400) + 'd ago';
}

function showNewDeployModal() {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.onclick = e => { if (e.target === overlay) overlay.remove(); };
    overlay.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Record Deployment</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Service *</label>
                        <input type="text" id="deploy-service" class="form-input" placeholder="my-api">
                    </div>
                    <div class="form-group">
                        <label class="form-label">Version *</label>
                        <input type="text" id="deploy-version" class="form-input" placeholder="v1.2.3">
                    </div>
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Environment</label>
                        <select id="deploy-env" class="form-select">
                            <option value="prod">Production</option>
                            <option value="staging">Staging</option>
                            <option value="dev">Development</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Status</label>
                        <select id="deploy-status" class="form-select">
                            <option value="success">Success</option>
                            <option value="failed">Failed</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-label">Deployed By</label>
                    <input type="text" id="deploy-user" class="form-input" placeholder="username">
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Commit SHA</label>
                        <input type="text" id="deploy-sha" class="form-input" placeholder="abc1234">
                    </div>
                    <div class="form-group">
                        <label class="form-label">Commit URL</label>
                        <input type="text" id="deploy-url" class="form-input" placeholder="https://github.com/...">
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-label">Description</label>
                    <input type="text" id="deploy-desc" class="form-input" placeholder="Optional notes">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="submitDeploy()">Record Deploy</button>
            </div>
        </div>
    `;
    document.body.appendChild(overlay);
}

function submitDeploy() {
    const service = document.getElementById('deploy-service').value.trim();
    const version = document.getElementById('deploy-version').value.trim();
    if (!service || !version) {
        alert('Service and Version are required');
        return;
    }

    const deploy = {
        service: service,
        version: version,
        environment: document.getElementById('deploy-env').value,
        status: document.getElementById('deploy-status').value,
        user: document.getElementById('deploy-user').value.trim(),
        commit_sha: document.getElementById('deploy-sha').value.trim(),
        commit_url: document.getElementById('deploy-url').value.trim(),
        description: document.getElementById('deploy-desc').value.trim()
    };

    fetch('/api/deploys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(deploy)
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to record deploy');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadDeployments();
    })
    .catch(err => alert('Error: ' + err.message));
}

function markDeployFailed(id) {
    if (!confirm('Mark this deployment as failed?')) return;
    fetch(`/api/deploys/${id}/status`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: 'failed' })
    })
    .then(() => loadDeployments())
    .catch(err => alert('Error: ' + err.message));
}

function markDeployRolledBack(id) {
    if (!confirm('Mark this deployment as rolled back?')) return;
    fetch(`/api/deploys/${id}/status`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: 'rolled_back' })
    })
    .then(() => loadDeployments())
    .catch(err => alert('Error: ' + err.message));
}

function showDeployImpact(id) {
    fetch(`/api/deploys/${id}/impact?window=30m`)
        .then(r => r.json())
        .then(impact => {
            fetch(`/api/deploys/${id}`)
                .then(r => r.json())
                .then(deploy => {
                    const impactColor = impact.impact === 'positive' ? '#00ba7c' :
                                      impact.impact === 'negative' ? '#f4212e' : '#71767b';
                    const impactIcon = impact.impact === 'positive' ? '&#9650;' :
                                     impact.impact === 'negative' ? '&#9660;' : '&#9679;';

                    const errorChange = impact.error_rate_change || 0;
                    const latencyChange = impact.latency_change || 0;
                    const errorColor = errorChange > 0 ? '#f4212e' : errorChange < 0 ? '#00ba7c' : '#71767b';
                    const latencyColor = latencyChange > 0 ? '#f4212e' : latencyChange < 0 ? '#00ba7c' : '#71767b';

                    const modal = document.createElement('div');
                    modal.className = 'modal-overlay';
                    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
                    modal.innerHTML = `
                        <div class="modal" style="max-width:500px">
                            <div class="modal-header">
                                <span class="modal-title">Deployment Impact Analysis</span>
                                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                            </div>
                            <div style="padding:1rem">
                                <div style="display:flex;align-items:center;gap:0.8rem;margin-bottom:1rem;padding-bottom:1rem;border-bottom:1px solid #2f3336">
                                    <div class="deploy-marker ${deploy.status}" style="height:50px"></div>
                                    <div>
                                        <div style="font-size:1.1rem;font-weight:600">${escapeHtml(deploy.service)} ${escapeHtml(deploy.version)}</div>
                                        <div style="color:#71767b;font-size:0.8rem">${new Date(deploy.timestamp).toLocaleString()}</div>
                                    </div>
                                    <div style="margin-left:auto;text-align:center">
                                        <div style="font-size:2rem;color:${impactColor}">${impactIcon}</div>
                                        <div style="font-size:0.7rem;text-transform:uppercase;font-weight:600;color:${impactColor}">${impact.impact}</div>
                                    </div>
                                </div>

                                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem">
                                    <div class="stat-card" style="padding:0.8rem">
                                        <div style="font-size:0.7rem;color:#71767b;text-transform:uppercase;margin-bottom:0.5rem">Error Rate Change</div>
                                        <div style="font-size:1.5rem;font-weight:700;color:${errorColor}">
                                            ${errorChange > 0 ? '+' : ''}${errorChange.toFixed(1)}%
                                        </div>
                                        <div style="font-size:0.7rem;color:#71767b;margin-top:0.3rem">
                                            ${impact.errors_before || 0} &rarr; ${impact.errors_after || 0} errors
                                        </div>
                                    </div>
                                    <div class="stat-card" style="padding:0.8rem">
                                        <div style="font-size:0.7rem;color:#71767b;text-transform:uppercase;margin-bottom:0.5rem">CPU Change</div>
                                        <div style="font-size:1.5rem;font-weight:700;color:${latencyColor}">
                                            ${latencyChange > 0 ? '+' : ''}${latencyChange.toFixed(1)}%
                                        </div>
                                        <div style="font-size:0.7rem;color:#71767b;margin-top:0.3rem">
                                            ${(impact.latency_p50_before || 0).toFixed(1)}% &rarr; ${(impact.latency_p50_after || 0).toFixed(1)}%
                                        </div>
                                    </div>
                                    <div class="stat-card" style="padding:0.8rem">
                                        <div style="font-size:0.7rem;color:#71767b;text-transform:uppercase;margin-bottom:0.5rem">Requests Before</div>
                                        <div style="font-size:1.5rem;font-weight:700">${impact.requests_before || 0}</div>
                                        <div style="font-size:0.7rem;color:#71767b;margin-top:0.3rem">30 min window</div>
                                    </div>
                                    <div class="stat-card" style="padding:0.8rem">
                                        <div style="font-size:0.7rem;color:#71767b;text-transform:uppercase;margin-bottom:0.5rem">Requests After</div>
                                        <div style="font-size:1.5rem;font-weight:700">${impact.requests_after || 0}</div>
                                        <div style="font-size:0.7rem;color:#71767b;margin-top:0.3rem">30 min window</div>
                                    </div>
                                </div>

                                ${deploy.description ? `
                                <div style="margin-top:1rem;padding:0.8rem;background:#1a1d21;border-radius:6px">
                                    <div style="font-size:0.7rem;color:#71767b;margin-bottom:0.3rem">Notes</div>
                                    <div style="font-size:0.85rem">${escapeHtml(deploy.description)}</div>
                                </div>` : ''}

                                ${deploy.commit_sha ? `
                                <div style="margin-top:0.8rem;font-size:0.8rem;color:#71767b">
                                    Commit: ${deploy.commit_url ?
                                        `<a href="${escapeHtml(deploy.commit_url)}" target="_blank" style="color:#1d9bf0">${deploy.commit_sha.substring(0,7)}</a>` :
                                        `<code>${deploy.commit_sha.substring(0,7)}</code>`}
                                    ${deploy.user ? ` by ${escapeHtml(deploy.user)}` : ''}
                                </div>` : ''}
                            </div>
                        </div>`;
                    document.body.appendChild(modal);
                });
        })
        .catch(err => alert('Error loading impact: ' + err.message));
}

function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// Incident functions
function loadIncidents() {
    fetch('/api/incidents?status=all&limit=20')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(incidents => {
            // Load stats
            fetch('/api/incidents/stats')
                .then(r => r.json())
                .then(stats => {
                    // Use demo stats if empty
                    if (DemoData.enabled && !stats?.active_incidents && !stats?.triggered_count) {
                        stats = DemoData.generateIncidentStats();
                    }
                    document.getElementById('inc-active').textContent = stats.active_incidents || 0;
                    document.getElementById('inc-triggered').textContent = stats.triggered_count || 0;
                    document.getElementById('inc-acked').textContent = stats.acknowledged_count || 0;
                })
                .catch(() => {
                    if (DemoData.enabled) {
                        const stats = DemoData.generateIncidentStats();
                        document.getElementById('inc-active').textContent = stats.active_incidents || 0;
                        document.getElementById('inc-triggered').textContent = stats.triggered_count || 0;
                        document.getElementById('inc-acked').textContent = stats.acknowledged_count || 0;
                    }
                });

            // Load on-call
            fetch('/api/oncall/current')
                .then(r => r.json())
                .then(oncall => {
                    // Use demo on-call if empty
                    if (DemoData.enabled && (!oncall || Object.keys(oncall).length === 0)) {
                        oncall = DemoData.generateCurrentOnCall();
                    }
                    const display = document.getElementById('oncall-display');
                    if (display) {
                        const names = Object.values(oncall).filter(v => v);
                        if (names.length > 0) {
                            display.innerHTML = `<span class="oncall-badge">${escapeHtml(names[0])}</span>`;
                        }
                    }
                })
                .catch(() => {
                    if (DemoData.enabled) {
                        const oncall = DemoData.generateCurrentOnCall();
                        const display = document.getElementById('oncall-display');
                        if (display) {
                            const names = Object.values(oncall).filter(v => v);
                            if (names.length > 0) {
                                display.innerHTML = `<span class="oncall-badge">${escapeHtml(names[0])}</span>`;
                            }
                        }
                    }
                });

            // Use demo data if empty and demo mode enabled
            if ((!incidents || incidents.length === 0) && DemoData.enabled) {
                incidents = DemoData.generateIncidents();
            }

            const list = document.getElementById('incidents-list');
            if (!list) return;

            if (!incidents || incidents.length === 0) {
                list.innerHTML = '<div class="empty-state">No incidents. All quiet!</div>';
                return;
            }

            list.innerHTML = incidents.map(inc => {
                const time = new Date(inc.created_at).toLocaleString();
                const relTime = formatRelativeTime(new Date(inc.created_at));
                const sourceLabel = inc.source ? `from ${inc.source}` : '';

                return `<div class="incident-item" onclick="showIncidentDetail('${inc.id}')">
                    <div class="incident-severity ${inc.severity || 'medium'}"></div>
                    <div class="incident-info">
                        <div class="incident-header">
                            <span class="incident-title">${escapeHtml(inc.title)}</span>
                            <span class="incident-status ${inc.status}">${inc.status}</span>
                        </div>
                        <div class="incident-meta">
                            <span class="incident-time" title="${time}">${relTime}</span>
                            ${inc.service ? `<span class="incident-source">${escapeHtml(inc.service)}</span>` : ''}
                            ${sourceLabel ? `<span class="incident-source">${sourceLabel}</span>` : ''}
                            ${inc.assigned_to ? `<span style="color:#00ba7c">@${escapeHtml(inc.assigned_to)}</span>` : ''}
                        </div>
                    </div>
                    <div class="incident-actions" onclick="event.stopPropagation()">
                        ${inc.status === 'triggered' ? `<button class="btn-quick ack" onclick="ackIncident('${inc.id}', true)" title="Quick Ack">Ack</button>` : ''}
                        ${inc.status !== 'resolved' ? `<button class="btn-quick resolve" onclick="resolveIncident('${inc.id}', true)" title="Quick Resolve">Resolve</button>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            if (DemoData.enabled) {
                // Load demo stats
                const stats = DemoData.generateIncidentStats();
                document.getElementById('inc-active').textContent = stats.active_incidents || 0;
                document.getElementById('inc-triggered').textContent = stats.triggered_count || 0;
                document.getElementById('inc-acked').textContent = stats.acknowledged_count || 0;
                // Load demo on-call
                const oncall = DemoData.generateCurrentOnCall();
                const display = document.getElementById('oncall-display');
                if (display) {
                    const names = Object.values(oncall).filter(v => v);
                    if (names.length > 0) display.innerHTML = `<span class="oncall-badge">${escapeHtml(names[0])}</span>`;
                }
                // Load demo incidents
                const list = document.getElementById('incidents-list');
                if (list) {
                    const incidents = DemoData.generateIncidents();
                    list.innerHTML = incidents.map(inc => {
                        const relTime = formatRelativeTime(new Date(inc.created_at));
                        return `<div class="incident-item"><div class="incident-severity ${inc.severity || 'medium'}"></div><div class="incident-info"><div class="incident-header"><span class="incident-title">${escapeHtml(inc.title)}</span><span class="incident-status ${inc.status}">${inc.status}</span></div><div class="incident-meta"><span class="incident-time">${relTime}</span>${inc.service ? `<span class="incident-source">${escapeHtml(inc.service)}</span>` : ''}</div></div></div>`;
                    }).join('');
                }
            } else {
                const list = document.getElementById('incidents-list');
                if (list) list.innerHTML = '<div class="empty-state">Incidents not available</div>';
            }
        });
}

function showNewIncidentModal() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Incident</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Title *</label>
                    <input type="text" id="inc-title" class="form-input" placeholder="Brief description of the incident">
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Severity</label>
                        <select id="inc-severity" class="form-select">
                            <option value="critical">Critical (P1)</option>
                            <option value="high">High (P2)</option>
                            <option value="medium" selected>Medium (P3)</option>
                            <option value="low">Low (P4)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Service</label>
                        <input type="text" id="inc-service" class="form-input" placeholder="affected-service">
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-label">Description</label>
                    <textarea id="inc-description" class="form-input" rows="3" placeholder="Details about the incident..."></textarea>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn btn-primary" onclick="submitIncident()">Create Incident</button>
            </div>
        </div>`;
    document.body.appendChild(modal);
}

function submitIncident() {
    const title = document.getElementById('inc-title').value.trim();
    if (!title) {
        alert('Title is required');
        return;
    }

    const incident = {
        title: title,
        severity: document.getElementById('inc-severity').value,
        service: document.getElementById('inc-service').value.trim(),
        description: document.getElementById('inc-description').value.trim(),
        source: 'manual'
    };

    fetch('/api/incidents', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(incident)
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to create incident');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadIncidents();
    })
    .catch(err => alert('Error: ' + err.message));
}

function ackIncident(id, skipModal = false) {
    if (skipModal) {
        // Quick ack with default user from localStorage
        const user = localStorage.getItem('dogwatch_user') || 'operator';
        fetch(`/api/incidents/${id}/ack`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ user: user })
        })
        .then(() => loadIncidents())
        .catch(err => showToast('Error: ' + err.message, 'error'));
        return;
    }

    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width:400px">
            <div class="modal-header">
                <span class="modal-title">Acknowledge Incident</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Your Name</label>
                    <input type="text" id="ack-user" value="${localStorage.getItem('dogwatch_user') || 'operator'}" placeholder="Enter your name">
                </div>
                <label style="display:flex;align-items:center;gap:0.5rem;font-size:0.8rem;color:#71767b;margin-top:0.5rem;">
                    <input type="checkbox" id="ack-remember" checked> Remember my name
                </label>
                <div id="ack-error" style="color:#f4212e;font-size:0.8rem;margin-top:0.5rem;display:none;"></div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="submitAck('${id}')">Acknowledge</button>
            </div>
        </div>`;
    document.body.appendChild(modal);
    document.getElementById('ack-user').focus();
    document.getElementById('ack-user').select();
}

function submitAck(id) {
    const user = document.getElementById('ack-user').value.trim();
    const remember = document.getElementById('ack-remember').checked;
    const errorEl = document.getElementById('ack-error');

    if (!user) {
        errorEl.textContent = 'Please enter your name';
        errorEl.style.display = 'block';
        return;
    }

    if (remember) {
        localStorage.setItem('dogwatch_user', user);
    }

    fetch(`/api/incidents/${id}/ack`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user: user })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to acknowledge');
        document.querySelector('.modal-overlay')?.remove();
        loadIncidents();
        showToast('Incident acknowledged', 'success');
    })
    .catch(err => {
        errorEl.textContent = err.message;
        errorEl.style.display = 'block';
    });
}

function resolveIncident(id, skipModal = false) {
    if (skipModal) {
        // Quick resolve with default user
        const user = localStorage.getItem('dogwatch_user') || 'operator';
        fetch(`/api/incidents/${id}/resolve`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ user: user, resolution: '' })
        })
        .then(() => loadIncidents())
        .catch(err => showToast('Error: ' + err.message, 'error'));
        return;
    }

    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width:450px">
            <div class="modal-header">
                <span class="modal-title">Resolve Incident</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Your Name</label>
                    <input type="text" id="resolve-user" value="${localStorage.getItem('dogwatch_user') || 'operator'}" placeholder="Enter your name">
                </div>
                <div class="form-group">
                    <label>Resolution Notes <span style="color:#71767b;font-weight:normal;">(optional)</span></label>
                    <textarea id="resolve-notes" rows="3" placeholder="What fixed it? Root cause?"></textarea>
                </div>
                <label style="display:flex;align-items:center;gap:0.5rem;font-size:0.8rem;color:#71767b;">
                    <input type="checkbox" id="resolve-remember" checked> Remember my name
                </label>
                <div id="resolve-error" style="color:#f4212e;font-size:0.8rem;margin-top:0.5rem;display:none;"></div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="submitResolve('${id}')">Resolve</button>
            </div>
        </div>`;
    document.body.appendChild(modal);
    document.getElementById('resolve-notes').focus();
}

function submitResolve(id) {
    const user = document.getElementById('resolve-user').value.trim();
    const resolution = document.getElementById('resolve-notes').value.trim();
    const remember = document.getElementById('resolve-remember').checked;
    const errorEl = document.getElementById('resolve-error');

    if (!user) {
        errorEl.textContent = 'Please enter your name';
        errorEl.style.display = 'block';
        return;
    }

    if (remember) {
        localStorage.setItem('dogwatch_user', user);
    }

    fetch(`/api/incidents/${id}/resolve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user: user, resolution: resolution })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to resolve');
        document.querySelector('.modal-overlay')?.remove();
        loadIncidents();
        showToast('Incident resolved', 'success');
    })
    .catch(err => {
        errorEl.textContent = err.message;
        errorEl.style.display = 'block';
    });
}

// Toast notification helper
function showToast(message, type = 'info') {
    const toast = document.createElement('div');
    toast.className = 'toast toast-' + type;
    toast.style.cssText = `
        position: fixed;
        bottom: 20px;
        right: 20px;
        padding: 12px 20px;
        border-radius: 6px;
        color: white;
        font-size: 0.9rem;
        z-index: 10001;
        animation: slideIn 0.3s ease;
        background: ${type === 'success' ? '#00ba7c' : type === 'error' ? '#f4212e' : '#1d9bf0'};
    `;
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(() => {
        toast.style.animation = 'fadeOut 0.3s ease';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

function showIncidentDetail(id) {
    fetch(`/api/incidents/${id}`)
        .then(r => r.json())
        .then(inc => {
            const severityColor = {
                critical: '#f4212e',
                high: '#ff6b35',
                medium: '#ffd400',
                low: '#1d9bf0'
            }[inc.severity] || '#71767b';

            const timeline = (inc.timeline || []).map(e => `
                <div style="display:flex;gap:0.5rem;padding:0.5rem 0;border-bottom:1px solid #2f3336;">
                    <div style="width:60px;flex-shrink:0;font-size:0.7rem;color:#71767b;">
                        ${new Date(e.timestamp).toLocaleTimeString()}
                    </div>
                    <div style="flex:1;">
                        <span style="font-weight:500;color:#e7e9ea;">${escapeHtml(e.type)}</span>
                        ${e.user ? `<span style="color:#71767b;"> by ${escapeHtml(e.user)}</span>` : ''}
                        <div style="font-size:0.8rem;color:#8b949e;margin-top:0.2rem;">${escapeHtml(e.message)}</div>
                    </div>
                </div>
            `).join('');

            const modal = document.createElement('div');
            modal.className = 'modal-overlay';
            modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
            modal.innerHTML = `
                <div class="modal" style="max-width:600px">
                    <div class="modal-header">
                        <span class="modal-title">Incident Details</span>
                        <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                    </div>
                    <div style="padding:1rem">
                        <div style="display:flex;gap:1rem;align-items:flex-start;margin-bottom:1rem;">
                            <div class="incident-severity ${inc.severity}" style="height:60px;width:6px;"></div>
                            <div style="flex:1;">
                                <div style="font-size:1.2rem;font-weight:600;margin-bottom:0.3rem;">${escapeHtml(inc.title)}</div>
                                <div style="display:flex;gap:0.5rem;align-items:center;">
                                    <span class="incident-status ${inc.status}">${inc.status}</span>
                                    <span style="color:${severityColor};font-weight:600;text-transform:uppercase;font-size:0.7rem;">${inc.severity}</span>
                                    ${inc.service ? `<span style="color:#71767b;">| ${escapeHtml(inc.service)}</span>` : ''}
                                </div>
                            </div>
                            <div style="text-align:right;">
                                ${inc.status === 'triggered' ? `<button class="btn btn-primary" onclick="ackIncident('${inc.id}');this.closest('.modal-overlay').remove();">Acknowledge</button>` : ''}
                                ${inc.status !== 'resolved' ? `<button class="btn" onclick="resolveIncident('${inc.id}');this.closest('.modal-overlay').remove();" style="margin-left:0.3rem;">Resolve</button>` : ''}
                            </div>
                        </div>

                        ${inc.description ? `
                        <div style="padding:0.8rem;background:#1a1d21;border-radius:6px;margin-bottom:1rem;">
                            <div style="font-size:0.7rem;color:#71767b;margin-bottom:0.3rem;">Description</div>
                            <div style="font-size:0.9rem;">${escapeHtml(inc.description)}</div>
                        </div>` : ''}

                        <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;">
                            <div class="stat-card" style="padding:0.6rem;">
                                <div style="font-size:0.65rem;color:#71767b;text-transform:uppercase;">Created</div>
                                <div style="font-size:0.85rem;">${new Date(inc.created_at).toLocaleString()}</div>
                            </div>
                            <div class="stat-card" style="padding:0.6rem;">
                                <div style="font-size:0.65rem;color:#71767b;text-transform:uppercase;">Assigned To</div>
                                <div style="font-size:0.85rem;">${inc.assigned_to || 'Unassigned'}</div>
                            </div>
                            ${inc.acked_at ? `
                            <div class="stat-card" style="padding:0.6rem;">
                                <div style="font-size:0.65rem;color:#71767b;text-transform:uppercase;">Acknowledged</div>
                                <div style="font-size:0.85rem;">${new Date(inc.acked_at).toLocaleString()} by ${inc.acked_by || 'unknown'}</div>
                            </div>` : ''}
                            ${inc.resolved_at ? `
                            <div class="stat-card" style="padding:0.6rem;">
                                <div style="font-size:0.65rem;color:#71767b;text-transform:uppercase;">Resolved</div>
                                <div style="font-size:0.85rem;">${new Date(inc.resolved_at).toLocaleString()} by ${inc.resolved_by || 'unknown'}</div>
                            </div>` : ''}
                        </div>

                        <div style="font-size:0.8rem;font-weight:600;margin-bottom:0.5rem;">Timeline</div>
                        <div style="max-height:200px;overflow:auto;">
                            ${timeline || '<div style="color:#71767b;">No timeline events</div>'}
                        </div>
                    </div>
                </div>`;
            document.body.appendChild(modal);
        })
        .catch(err => alert('Error loading incident: ' + err.message));
}

// Initialize SLOs, Patterns, Containers, Deployments, and Incidents
setTimeout(loadSLOs, 600);
setInterval(loadSLOs, 30000);
setTimeout(loadPatterns, 700);
setInterval(loadPatterns, 60000);
setTimeout(loadContainers, 800);
setInterval(loadContainers, 30000);
setTimeout(loadDeployments, 900);
setInterval(loadDeployments, 30000);
setTimeout(loadIncidents, 1000);
setInterval(loadIncidents, 30000);  // WebSocket handles real-time updates

// ============ Cluster/Federation Functions ============
function loadCluster() {
    fetch('/api/cluster')
        .then(r => r.json())
        .then(info => {
            const enabledEl = document.getElementById('cluster-enabled');
            const nodesCountEl = document.getElementById('cluster-nodes');
            const localEl = document.getElementById('cluster-local');
            const nodesListEl = document.getElementById('cluster-nodes-list');

            if (!enabledEl) return;

            // Use demo data if not enabled and demo mode is on
            if (!info.enabled && DemoData.enabled) {
                info = DemoData.generateClusterInfo();
            }

            if (!info.enabled) {
                enabledEl.textContent = 'Disabled';
                enabledEl.style.color = '#71767b';
                nodesCountEl.textContent = '-';
                localEl.textContent = '';
                nodesListEl.innerHTML = '<div class="empty-state">Federation not enabled. Start with --cluster flag.</div>';
                return;
            }

            enabledEl.textContent = 'Active';
            enabledEl.style.color = '#00ba7c';
            nodesCountEl.textContent = info.node_count;
            nodesCountEl.style.color = info.node_count > 1 ? '#00ba7c' : '#e7e9ea';

            if (info.local_node) {
                localEl.textContent = `This node: ${info.local_node.id} (${info.gossip_addr})`;
            }

            // Load node list (or use demo data)
            if (DemoData.enabled) {
                return Promise.resolve(DemoData.generateClusterNodes());
            }
            return fetch('/api/cluster/nodes').then(r => r.json());
        })
        .then(nodes => {
            if (!nodes) return;

            const nodesListEl = document.getElementById('cluster-nodes-list');
            if (!nodesListEl) return;

            if (!nodes || nodes.length === 0) {
                nodesListEl.innerHTML = '<div class="empty-state">No nodes in cluster</div>';
                return;
            }

            // Get local node ID for comparison
            let localNodeId = DemoData.enabled ? 'node-local-001' : '';
            if (!DemoData.enabled) {
                fetch('/api/cluster').then(r => r.json()).then(info => {
                    localNodeId = info.node_id || '';
                });
            }

            nodesListEl.innerHTML = nodes.map(node => {
                const isLocal = node.id === localNodeId || (node.hostname && node.hostname === location.hostname);
                const stateClass = (node.state || 'alive').toLowerCase();
                const uptime = node.started_at ? formatUptime(new Date(node.started_at)) : '-';

                return `<div class="cluster-node-item">
                    <div class="cluster-node-status ${stateClass}"></div>
                    <div class="cluster-node-info">
                        <div class="cluster-node-name">
                            ${node.id || node.hostname || 'unknown'}
                            ${isLocal ? '<span class="local-badge">LOCAL</span>' : ''}
                        </div>
                        <div class="cluster-node-meta">
                            <span>${node.address || '-'}</span>
                            <span>v${node.version || '?'}</span>
                            <span>Up ${uptime}</span>
                        </div>
                    </div>
                    <div class="cluster-node-metrics">
                        <div class="cluster-node-metric">
                            <span class="cluster-node-metric-value">${node.cpu_percent ? node.cpu_percent.toFixed(1) + '%' : '-'}</span>
                            <span>CPU</span>
                        </div>
                        <div class="cluster-node-metric">
                            <span class="cluster-node-metric-value">${node.mem_percent ? node.mem_percent.toFixed(1) + '%' : '-'}</span>
                            <span>Mem</span>
                        </div>
                        <div class="cluster-node-metric">
                            <span class="cluster-node-metric-value">${node.active_incidents || 0}</span>
                            <span>Inc</span>
                        </div>
                    </div>
                </div>`;
            }).join('');

            // Update cluster incidents count
            const totalIncidents = nodes.reduce((sum, n) => sum + (n.active_incidents || 0), 0);
            const incEl = document.getElementById('cluster-incidents');
            if (incEl) {
                incEl.textContent = totalIncidents;
                incEl.style.color = totalIncidents > 0 ? '#f4212e' : '#e7e9ea';
            }
        })
        .catch(err => console.error('Error loading cluster:', err));
}

function formatUptime(startTime) {
    const now = new Date();
    const diff = now - startTime;
    const days = Math.floor(diff / (24 * 60 * 60 * 1000));
    const hours = Math.floor((diff % (24 * 60 * 60 * 1000)) / (60 * 60 * 1000));
    const mins = Math.floor((diff % (60 * 60 * 1000)) / (60 * 1000));

    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${mins}m`;
    return `${mins}m`;
}

function showJoinClusterModal() {
    const addresses = prompt('Enter seed node addresses (comma-separated, e.g., 192.168.1.10:7946,192.168.1.11:7946):');
    if (!addresses) return;

    fetch('/api/cluster/join', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ addresses: addresses.split(',').map(a => a.trim()) })
    })
    .then(r => {
        if (!r.ok) return r.text().then(t => { throw new Error(t); });
        return r.json();
    })
    .then(result => {
        alert(`Joined ${result.joined} node(s). Total nodes: ${result.total}`);
        loadCluster();
    })
    .catch(err => alert('Error joining cluster: ' + err.message));
}

setTimeout(loadCluster, 1100);
setInterval(loadCluster, 10000);

// ============ Status Page Functions ============
// moved to top
// moved to top

function loadStatusPages() {
    Promise.all([
        fetch('/api/statuspages').then(r => r.json()),
        fetch('/api/statuspage/components').then(r => r.json()),
        fetch('/api/statuspage/incidents').then(r => r.json())
    ])
    .then(([pages, components, incidents]) => {
        statusPages = pages || [];
        statusComponents = components || [];

        const pagesEl = document.getElementById('sp-pages');
        const componentsEl = document.getElementById('sp-components');
        const incidentsEl = document.getElementById('sp-incidents');
        const overallEl = document.getElementById('statuspage-overall');
        const listEl = document.getElementById('statuspage-list');

        if (!listEl) return;

        if (pagesEl) pagesEl.textContent = statusPages.length;
        if (componentsEl) componentsEl.textContent = statusComponents.length;
        if (incidentsEl) {
            const activeIncidents = (incidents || []).filter(i => i.status !== 'resolved').length;
            incidentsEl.textContent = activeIncidents;
            if (activeIncidents > 0) incidentsEl.style.color = '#f4212e';
            else incidentsEl.style.color = '#e7e9ea';
        }

        // Calculate overall status from components
        if (overallEl) {
            const statuses = statusComponents.map(c => c.status);
            let overallStatus = 'operational';
            let overallText = 'All Systems Operational';

            if (statuses.includes('major_outage')) {
                overallStatus = 'major_outage';
                overallText = 'Major Outage';
            } else if (statuses.includes('partial_outage')) {
                overallStatus = 'partial_outage';
                overallText = 'Partial Outage';
            } else if (statuses.includes('degraded')) {
                overallStatus = 'degraded';
                overallText = 'Degraded Performance';
            } else if (statuses.includes('maintenance')) {
                overallStatus = 'maintenance';
                overallText = 'Under Maintenance';
            }

            overallEl.className = 'statuspage-overall ' + overallStatus;
            overallEl.textContent = overallText;
        }

        // Render components grouped by group
        if (statusComponents.length === 0 && statusPages.length === 0) {
            listEl.innerHTML = '<div class="empty-state">No status pages configured. Click "+ New Page" to create one.</div>';
            return;
        }

        let html = '';

        // Group components
        const groups = {};
        statusComponents.forEach(c => {
            const group = c.group || 'Services';
            if (!groups[group]) groups[group] = [];
            groups[group].push(c);
        });

        // Render grouped components
        Object.keys(groups).sort().forEach(groupName => {
            html += `<div class="component-group">${groupName}</div>`;
            groups[groupName].forEach(comp => {
                html += renderComponentItem(comp);
            });
        });

        // Render status pages
        if (statusPages.length > 0) {
            html += `<div class="component-group">Status Pages</div>`;
            statusPages.forEach(page => {
                html += `<div class="statuspage-item" onclick="viewStatusPage('${page.slug}')">
                    <div class="statuspage-status operational"></div>
                    <div class="statuspage-info">
                        <div class="statuspage-name">${escapeHtml(page.name)}</div>
                        <div class="statuspage-meta">
                            <span>${page.slug}</span>
                            <span>${page.public ? 'Public' : 'Private'}</span>
                            <span>${page.component_ids ? page.component_ids.length : 0} components</span>
                        </div>
                    </div>
                    <div style="display:flex;gap:0.3rem;">
                        <button class="btn" onclick="event.stopPropagation();editStatusPage('${page.id}')">Edit</button>
                        <button class="btn" onclick="event.stopPropagation();window.open('/status/${page.slug}','_blank')">View</button>
                    </div>
                </div>`;
            });
        }

        listEl.innerHTML = html || '<div class="empty-state">No components configured</div>';
    })
    .catch(err => console.error('Error loading status pages:', err));
}

function renderComponentItem(comp) {
    const statusClass = comp.status || 'operational';
    const uptime = comp.uptime_30d ? comp.uptime_30d.toFixed(2) : '100.00';

    return `<div class="component-item" onclick="showComponentDetail('${comp.id}')">
        <div class="statuspage-status ${statusClass}"></div>
        <div class="statuspage-info">
            <div class="statuspage-name">${escapeHtml(comp.name)}</div>
            <div class="statuspage-meta">
                <span>${comp.description || ''}</span>
            </div>
        </div>
        <div class="statuspage-uptime">${uptime}%</div>
        <div style="display:flex;gap:0.3rem;">
            <button class="btn" onclick="event.stopPropagation();updateComponentStatus('${comp.id}')">Update</button>
        </div>
    </div>`;
}

function showCreateStatusPage() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Status Page</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Name</label>
                    <input type="text" id="sp-name" placeholder="My Service Status">
                </div>
                <div class="form-group">
                    <label>Slug (URL path)</label>
                    <input type="text" id="sp-slug" placeholder="my-service">
                </div>
                <div class="form-group">
                    <label>Description</label>
                    <textarea id="sp-description" rows="2" placeholder="Status page for my service"></textarea>
                </div>
                <div class="form-group">
                    <label><input type="checkbox" id="sp-public" checked> Public (accessible without login)</label>
                </div>
                <div class="form-group">
                    <label><input type="checkbox" id="sp-uptime" checked> Show uptime metrics</label>
                </div>
                <div class="form-group">
                    <label>Components</label>
                    <div id="sp-components-list" style="max-height:150px;overflow:auto;">
                        ${statusComponents.map(c => `
                            <label style="display:block;padding:0.3rem 0;">
                                <input type="checkbox" value="${c.id}" class="sp-comp-check"> ${escapeHtml(c.name)}
                            </label>
                        `).join('') || '<span class="empty-state">No components. Create components first.</span>'}
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createStatusPage()">Create</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createStatusPage() {
    const name = document.getElementById('sp-name').value;
    const slug = document.getElementById('sp-slug').value;
    const description = document.getElementById('sp-description').value;
    const isPublic = document.getElementById('sp-public').checked;
    const showUptime = document.getElementById('sp-uptime').checked;
    const componentIds = Array.from(document.querySelectorAll('.sp-comp-check:checked')).map(c => c.value);

    if (!name || !slug) {
        alert('Name and slug are required');
        return;
    }

    fetch('/api/statuspages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            name, slug, description,
            public: isPublic,
            show_uptime: showUptime,
            show_incidents: true,
            component_ids: componentIds
        })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to create status page');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadStatusPages();
    })
    .catch(err => alert('Error: ' + err.message));
}

function showCreateComponent() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Component</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Name</label>
                    <input type="text" id="comp-name" placeholder="API Server">
                </div>
                <div class="form-group">
                    <label>Description</label>
                    <input type="text" id="comp-description" placeholder="Core API services">
                </div>
                <div class="form-group">
                    <label>Group</label>
                    <input type="text" id="comp-group" placeholder="Infrastructure" value="Services">
                </div>
                <div class="form-group">
                    <label>Initial Status</label>
                    <select id="comp-status">
                        <option value="operational">Operational</option>
                        <option value="degraded">Degraded</option>
                        <option value="partial_outage">Partial Outage</option>
                        <option value="major_outage">Major Outage</option>
                        <option value="maintenance">Maintenance</option>
                    </select>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createComponent()">Create</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createComponent() {
    const name = document.getElementById('comp-name').value;
    const description = document.getElementById('comp-description').value;
    const group = document.getElementById('comp-group').value;
    const status = document.getElementById('comp-status').value;

    if (!name) {
        alert('Name is required');
        return;
    }

    fetch('/api/statuspage/components', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, description, group, status })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to create component');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadStatusPages();
    })
    .catch(err => alert('Error: ' + err.message));
}

function updateComponentStatus(compId) {
    const comp = statusComponents.find(c => c.id === compId);
    if (!comp) return;

    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Update Status: ${escapeHtml(comp.name)}</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Status</label>
                    <select id="update-status">
                        <option value="operational" ${comp.status === 'operational' ? 'selected' : ''}>Operational</option>
                        <option value="degraded" ${comp.status === 'degraded' ? 'selected' : ''}>Degraded</option>
                        <option value="partial_outage" ${comp.status === 'partial_outage' ? 'selected' : ''}>Partial Outage</option>
                        <option value="major_outage" ${comp.status === 'major_outage' ? 'selected' : ''}>Major Outage</option>
                        <option value="maintenance" ${comp.status === 'maintenance' ? 'selected' : ''}>Maintenance</option>
                    </select>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="submitStatusUpdate('${compId}')">Update</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function submitStatusUpdate(compId) {
    const status = document.getElementById('update-status').value;

    fetch(`/api/statuspage/components/${compId}/status`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status, response_time_ms: 0 })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to update status');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadStatusPages();
    })
    .catch(err => alert('Error: ' + err.message));
}

function viewStatusPage(slug) {
    window.open('/status/' + slug, '_blank');
}

function editStatusPage(id) {
    const page = statusPages.find(p => p.id === id);
    if (!page) return;

    // For simplicity, just show the basic info - could expand this
    alert('Edit functionality coming soon. View the page at: /status/' + page.slug);
}

function showComponentDetail(compId) {
    const comp = statusComponents.find(c => c.id === compId);
    if (!comp) return;

    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">${escapeHtml(comp.name)}</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body">
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Current Status</div>
                        <div style="font-size:1.2rem;font-weight:600;color:${getStatusColor(comp.status)}">${formatStatus(comp.status)}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Response Time</div>
                        <div style="font-size:1.2rem;font-weight:600;">${comp.response_time_ms ? comp.response_time_ms.toFixed(0) + 'ms' : '-'}</div>
                    </div>
                </div>
                <div style="margin-top:1rem;">
                    <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Uptime</div>
                    <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:0.5rem;text-align:center;">
                        <div>
                            <div style="font-size:1rem;font-weight:600;color:#00ba7c">${(comp.uptime_24h || 100).toFixed(2)}%</div>
                            <div style="font-size:0.65rem;color:#71767b">24h</div>
                        </div>
                        <div>
                            <div style="font-size:1rem;font-weight:600;color:#00ba7c">${(comp.uptime_7d || 100).toFixed(2)}%</div>
                            <div style="font-size:0.65rem;color:#71767b">7d</div>
                        </div>
                        <div>
                            <div style="font-size:1rem;font-weight:600;color:#00ba7c">${(comp.uptime_30d || 100).toFixed(2)}%</div>
                            <div style="font-size:0.65rem;color:#71767b">30d</div>
                        </div>
                        <div>
                            <div style="font-size:1rem;font-weight:600;color:#00ba7c">${(comp.uptime_90d || 100).toFixed(2)}%</div>
                            <div style="font-size:0.65rem;color:#71767b">90d</div>
                        </div>
                    </div>
                </div>
                ${comp.description ? `<div style="margin-top:1rem;color:#71767b;font-size:0.8rem;">${escapeHtml(comp.description)}</div>` : ''}
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                <button class="btn btn-primary" onclick="this.closest('.modal-overlay').remove();updateComponentStatus('${compId}')">Update Status</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function getStatusColor(status) {
    switch(status) {
        case 'operational': return '#00ba7c';
        case 'degraded': return '#ffd400';
        case 'partial_outage': return '#ff9800';
        case 'major_outage': return '#f4212e';
        case 'maintenance': return '#1d9bf0';
        default: return '#71767b';
    }
}

function formatStatus(status) {
    switch(status) {
        case 'operational': return 'Operational';
        case 'degraded': return 'Degraded';
        case 'partial_outage': return 'Partial Outage';
        case 'major_outage': return 'Major Outage';
        case 'maintenance': return 'Maintenance';
        default: return 'Unknown';
    }
}

setTimeout(loadStatusPages, 1200);
setInterval(loadStatusPages, 30000);

// ============ Service Catalog Functions ============
// moved to top
// moved to top

function loadCatalog() {
    const tierFilter = document.getElementById('cat-tier-filter')?.value || '';
    const healthFilter = document.getElementById('cat-health-filter')?.value || '';

    let url = '/api/catalog/services?';
    if (tierFilter) url += `tier=${tierFilter}&`;
    if (healthFilter) url += `health=${healthFilter}&`;

    Promise.all([
        fetch(url).then(r => r.json()),
        fetch('/api/catalog/services/stats').then(r => r.json()),
        fetch('/api/catalog/teams').then(r => r.json())
    ])
    .then(([services, stats, teams]) => {
        // Use demo data if empty and demo mode enabled
        if (DemoData.enabled && (!services || services.length === 0)) {
            services = DemoData.generateCatalogServices();
            stats = DemoData.generateCatalogStats();
            teams = DemoData.generateCatalogTeams();
        }
        catalogServices = services || [];
        catalogTeams = teams || [];

        const totalEl = document.getElementById('cat-total');
        const criticalEl = document.getElementById('cat-critical');
        const healthyEl = document.getElementById('cat-healthy');
        const unhealthyEl = document.getElementById('cat-unhealthy');
        const listEl = document.getElementById('catalog-list');

        if (!listEl) return;

        if (totalEl) totalEl.textContent = stats.total || 0;
        if (criticalEl) criticalEl.textContent = stats.critical || 0;
        if (healthyEl) healthyEl.textContent = stats.healthy || 0;
        if (unhealthyEl) unhealthyEl.textContent = stats.unhealthy || 0;

        if (catalogServices.length === 0) {
            listEl.innerHTML = '<div class="empty-state">No services in catalog. Click "+ Service" to add one.</div>';
            return;
        }

        listEl.innerHTML = catalogServices.map(svc => renderServiceItem(svc)).join('');
    })
    .catch(err => console.error('Error loading catalog:', err));
}

function renderServiceItem(svc) {
    const healthClass = svc.health || 'unknown';
    const tierClass = svc.tier || 'medium';

    let links = '';
    if (svc.repo_url) links += `<a class="service-link" href="${svc.repo_url}" target="_blank" onclick="event.stopPropagation()">Repo</a>`;
    if (svc.docs_url) links += `<a class="service-link" href="${svc.docs_url}" target="_blank" onclick="event.stopPropagation()">Docs</a>`;
    if (svc.runbook_url) links += `<a class="service-link" href="${svc.runbook_url}" target="_blank" onclick="event.stopPropagation()">Runbook</a>`;

    return `<div class="service-item" onclick="showServiceDetail('${svc.id}')">
        <div class="service-health ${healthClass}"></div>
        <div class="service-info">
            <div class="service-name">
                ${escapeHtml(svc.display_name || svc.name)}
                <span class="service-tier ${tierClass}">${svc.tier || 'medium'}</span>
            </div>
            <div class="service-meta">
                ${svc.team_name ? `<span class="service-owner"><span class="team-badge">${escapeHtml(svc.team_name)}</span></span>` : ''}
                ${svc.description ? `<span>${escapeHtml(svc.description.substring(0, 50))}${svc.description.length > 50 ? '...' : ''}</span>` : ''}
            </div>
        </div>
        <div class="service-links">${links}</div>
    </div>`;
}

function showCreateService() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal" style="max-width:600px;">
            <div class="modal-header">
                <span class="modal-title">Add Service</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body">
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
                    <div class="form-group">
                        <label>Service Name *</label>
                        <input type="text" id="svc-name" placeholder="payment-service">
                    </div>
                    <div class="form-group">
                        <label>Display Name</label>
                        <input type="text" id="svc-display" placeholder="Payment Service">
                    </div>
                </div>
                <div class="form-group">
                    <label>Description</label>
                    <textarea id="svc-desc" rows="2" placeholder="Handles payment processing"></textarea>
                </div>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
                    <div class="form-group">
                        <label>Tier</label>
                        <select id="svc-tier">
                            <option value="critical">Critical (P0)</option>
                            <option value="high">High (P1)</option>
                            <option value="medium" selected>Medium (P2)</option>
                            <option value="low">Low (P3)</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>Team</label>
                        <select id="svc-team">
                            <option value="">No Team</option>
                            ${catalogTeams.map(t => `<option value="${t.id}">${escapeHtml(t.name)}</option>`).join('')}
                        </select>
                    </div>
                </div>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
                    <div class="form-group">
                        <label>Owner Email</label>
                        <input type="email" id="svc-owner" placeholder="team@example.com">
                    </div>
                    <div class="form-group">
                        <label>Slack Channel</label>
                        <input type="text" id="svc-slack" placeholder="#team-payments">
                    </div>
                </div>
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
                    <div class="form-group">
                        <label>Repository URL</label>
                        <input type="url" id="svc-repo" placeholder="https://github.com/...">
                    </div>
                    <div class="form-group">
                        <label>Documentation URL</label>
                        <input type="url" id="svc-docs" placeholder="https://docs.example.com/...">
                    </div>
                </div>
                <div class="form-group">
                    <label>Runbook URL</label>
                    <input type="url" id="svc-runbook" placeholder="https://runbooks.example.com/...">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createService()">Create Service</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createService() {
    const name = document.getElementById('svc-name').value;
    const displayName = document.getElementById('svc-display').value;
    const description = document.getElementById('svc-desc').value;
    const tier = document.getElementById('svc-tier').value;
    const teamId = document.getElementById('svc-team').value;
    const ownerEmail = document.getElementById('svc-owner').value;
    const slackChannel = document.getElementById('svc-slack').value;
    const repoUrl = document.getElementById('svc-repo').value;
    const docsUrl = document.getElementById('svc-docs').value;
    const runbookUrl = document.getElementById('svc-runbook').value;

    if (!name) {
        alert('Service name is required');
        return;
    }

    const teamName = teamId ? catalogTeams.find(t => t.id === teamId)?.name : '';

    fetch('/api/catalog/services', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            name,
            display_name: displayName || name,
            description,
            tier,
            team_id: teamId,
            team_name: teamName,
            owner_email: ownerEmail,
            slack_channel: slackChannel,
            repo_url: repoUrl,
            docs_url: docsUrl,
            runbook_url: runbookUrl
        })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to create service');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadCatalog();
    })
    .catch(err => alert('Error: ' + err.message));
}

function showCreateTeam() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Team</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Team Name *</label>
                    <input type="text" id="team-name" placeholder="Platform Team">
                </div>
                <div class="form-group">
                    <label>Description</label>
                    <textarea id="team-desc" rows="2" placeholder="Responsible for core platform services"></textarea>
                </div>
                <div class="form-group">
                    <label>Slack Channel</label>
                    <input type="text" id="team-slack" placeholder="#platform-team">
                </div>
                <div class="form-group">
                    <label>Email</label>
                    <input type="email" id="team-email" placeholder="platform@example.com">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createTeam()">Create Team</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createTeam() {
    const name = document.getElementById('team-name').value;
    const description = document.getElementById('team-desc').value;
    const slackChannel = document.getElementById('team-slack').value;
    const email = document.getElementById('team-email').value;

    if (!name) {
        alert('Team name is required');
        return;
    }

    fetch('/api/catalog/teams', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, description, slack_channel: slackChannel, email })
    })
    .then(r => {
        if (!r.ok) throw new Error('Failed to create team');
        return r.json();
    })
    .then(() => {
        document.querySelector('.modal-overlay').remove();
        loadCatalog();
    })
    .catch(err => alert('Error: ' + err.message));
}

function showServiceDetail(serviceId) {
    const svc = catalogServices.find(s => s.id === serviceId);
    if (!svc) return;

    // Fetch full service context with linked resources
    fetch(`/api/catalog/services/${serviceId}/context`)
        .then(r => r.json())
        .then(ctx => {
            showServiceDetailModal(ctx.service || svc, {
                upstream: ctx.upstream_dependencies || [],
                downstream: ctx.downstream_dependencies || [],
                runbooks: ctx.runbooks || [],
                incidents: ctx.recent_incidents || [],
                synthetics: ctx.synthetic_checks || [],
                team: ctx.team
            });
        })
        .catch(() => showServiceDetailModal(svc, { upstream: [], downstream: [], runbooks: [], incidents: [], synthetics: [] }));
}

function showServiceDetailModal(svc, ctx) {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };

    const upstreamHtml = (ctx.upstream || []).length > 0
        ? ctx.upstream.map(d => `<span class="service-link">${d.target_service_id}</span>`).join(' ')
        : '<span style="color:#71767b">None</span>';

    const downstreamHtml = (ctx.downstream || []).length > 0
        ? ctx.downstream.map(d => `<span class="service-link">${d.source_service_id}</span>`).join(' ')
        : '<span style="color:#71767b">None</span>';

    // Build runbooks section
    const runbooksHtml = (ctx.runbooks || []).length > 0
        ? ctx.runbooks.map(rb => `<a class="service-link" href="${rb.content_url || '#'}" target="_blank">${escapeHtml(rb.title)}</a>`).join(' ')
        : '<span style="color:#71767b">None</span>';

    // Build recent incidents section
    const incidentsHtml = (ctx.incidents || []).length > 0
        ? ctx.incidents.slice(0, 5).map(inc => {
            const sevClass = inc.severity === 'critical' ? 'severity-critical' :
                             inc.severity === 'high' ? 'severity-high' :
                             inc.severity === 'medium' ? 'severity-medium' : 'severity-low';
            const statusColor = inc.status === 'resolved' ? '#00ba7c' : inc.status === 'acknowledged' ? '#ffd400' : '#f4212e';
            return `<div style="display:flex;align-items:center;gap:0.5rem;padding:0.3rem 0;border-bottom:1px solid #2f3336;">
                <span class="severity-badge ${sevClass}">${inc.severity}</span>
                <span style="flex:1;font-size:0.8rem;">${escapeHtml(inc.title || 'Incident')}</span>
                <span style="font-size:0.7rem;color:${statusColor}">${inc.status}</span>
            </div>`;
        }).join('')
        : '<span style="color:#71767b">No recent incidents</span>';

    // Build synthetic checks section
    const syntheticsHtml = (ctx.synthetics || []).length > 0
        ? ctx.synthetics.map(chk => {
            const statusColor = chk.passing ? '#00ba7c' : '#f4212e';
            const statusText = chk.passing ? 'Passing' : 'Failing';
            return `<div style="display:flex;align-items:center;gap:0.5rem;padding:0.3rem 0;border-bottom:1px solid #2f3336;">
                <span style="width:8px;height:8px;border-radius:50%;background:${statusColor};"></span>
                <span style="flex:1;font-size:0.8rem;">${escapeHtml(chk.name)}</span>
                <span style="font-size:0.7rem;color:#71767b;">${chk.check_type}</span>
                <span style="font-size:0.7rem;color:${statusColor}">${statusText}</span>
            </div>`;
        }).join('')
        : '<span style="color:#71767b">No synthetic checks</span>';

    modal.innerHTML = `
        <div class="modal" style="max-width:800px;">
            <div class="modal-header">
                <span class="modal-title">${escapeHtml(svc.display_name || svc.name)}</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body" style="max-height:70vh;overflow-y:auto;">
                <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:1rem;margin-bottom:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Health</div>
                        <div style="font-size:1.2rem;font-weight:600;color:${getHealthColor(svc.health)}">${formatHealth(svc.health)}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Tier</div>
                        <div style="font-size:1.2rem;font-weight:600;">${(svc.tier || 'medium').toUpperCase()}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Uptime (30d)</div>
                        <div style="font-size:1.2rem;font-weight:600;color:#00ba7c">${(svc.uptime_percent_30d || 100).toFixed(2)}%</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Incidents (30d)</div>
                        <div style="font-size:1.2rem;font-weight:600;color:${svc.incident_count_30d > 0 ? '#f4212e' : '#e7e9ea'}">${svc.incident_count_30d || 0}</div>
                    </div>
                </div>

                ${svc.description ? `<p style="color:#71767b;margin-bottom:1rem;">${escapeHtml(svc.description)}</p>` : ''}

                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Ownership</div>
                        ${ctx.team ? `<div><span class="team-badge">${escapeHtml(ctx.team.name)}</span></div>` :
                          svc.team_name ? `<div><span class="team-badge">${escapeHtml(svc.team_name)}</span></div>` : ''}
                        ${svc.owner_email ? `<div style="font-size:0.8rem;margin-top:0.3rem;">${escapeHtml(svc.owner_email)}</div>` : ''}
                        ${svc.slack_channel ? `<div style="font-size:0.8rem;color:#71767b;">${escapeHtml(svc.slack_channel)}</div>` : ''}
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Links</div>
                        <div style="display:flex;flex-wrap:wrap;gap:0.3rem;">
                            ${svc.repo_url ? `<a class="service-link" href="${svc.repo_url}" target="_blank">Repository</a>` : ''}
                            ${svc.docs_url ? `<a class="service-link" href="${svc.docs_url}" target="_blank">Documentation</a>` : ''}
                            ${svc.runbook_url ? `<a class="service-link" href="${svc.runbook_url}" target="_blank">Runbook</a>` : ''}
                            ${svc.dashboard_id ? `<a class="service-link" href="#" onclick="event.preventDefault();">Dashboard</a>` : ''}
                        </div>
                    </div>
                </div>

                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Depends On (Upstream)</div>
                        <div>${upstreamHtml}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Used By (Downstream)</div>
                        <div>${downstreamHtml}</div>
                    </div>
                </div>

                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Runbooks</div>
                        <div style="display:flex;flex-wrap:wrap;gap:0.3rem;">${runbooksHtml}</div>
                    </div>
                </div>

                <div style="margin-bottom:1rem;padding:0.75rem;background:#16181c;border-radius:8px;">
                    <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Recent Incidents</div>
                    <div>${incidentsHtml}</div>
                </div>

                <div style="margin-bottom:1rem;padding:0.75rem;background:#16181c;border-radius:8px;">
                    <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Synthetic Checks</div>
                    <div>${syntheticsHtml}</div>
                </div>

                ${svc.k8s_namespace ? `
                <div style="margin-top:1rem;padding-top:1rem;border-top:1px solid #2f3336;">
                    <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Kubernetes</div>
                    <div style="font-size:0.8rem;">
                        <span style="color:#71767b;">Namespace:</span> ${escapeHtml(svc.k8s_namespace)}
                        ${svc.k8s_deployment ? ` | <span style="color:#71767b;">Deployment:</span> ${escapeHtml(svc.k8s_deployment)}` : ''}
                    </div>
                </div>
                ` : ''}
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                <button class="btn btn-primary" onclick="this.closest('.modal-overlay').remove();editService('${svc.id}')">Edit</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function getHealthColor(health) {
    switch(health) {
        case 'healthy': return '#00ba7c';
        case 'degraded': return '#ffd400';
        case 'unhealthy': return '#f4212e';
        default: return '#71767b';
    }
}

function formatHealth(health) {
    switch(health) {
        case 'healthy': return 'Healthy';
        case 'degraded': return 'Degraded';
        case 'unhealthy': return 'Unhealthy';
        default: return 'Unknown';
    }
}

function editService(serviceId) {
    alert('Edit functionality coming soon');
}

setTimeout(loadCatalog, 1300);
setInterval(loadCatalog, 30000);

// ============ Correlation Engine Functions ============
// moved to top

function loadCorrelations() {
    const timeRange = document.getElementById('corr-timerange')?.value || '24h';

    // Load deploy-incident correlations
    fetch(`/api/correlate/deploy-incidents?since=${timeRange}`)
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(data => {
            // Use demo data if empty and demo mode enabled
            if (DemoData.enabled && (!data?.correlations || data.correlations.length === 0)) {
                data = DemoData.generateCorrelations();
            }
            correlationData.correlations = data.correlations || [];

            const countEl = document.getElementById('corr-deploy-incidents');
            const listEl = document.getElementById('corr-deploy-list');

            if (countEl) countEl.textContent = correlationData.correlations.length;

            if (listEl) {
                if (correlationData.correlations.length === 0) {
                    listEl.innerHTML = '<div class="empty-state" style="padding:1rem;text-align:center;color:#666;">No deploy→incident correlations detected</div>';
                } else {
                    listEl.innerHTML = correlationData.correlations.map(corr => renderCorrelation(corr)).join('');
                }
            }
        })
        .catch(err => {
            if (DemoData.enabled) {
                const data = DemoData.generateCorrelations();
                correlationData.correlations = data.correlations || [];
                const countEl = document.getElementById('corr-deploy-incidents');
                const listEl = document.getElementById('corr-deploy-list');
                if (countEl) countEl.textContent = correlationData.correlations.length;
                if (listEl) listEl.innerHTML = correlationData.correlations.map(corr => renderCorrelation(corr)).join('');
            } else {
                console.error('Error loading correlations:', err);
            }
        });

    // Load services for dropdown
    fetch('/api/catalog/services')
        .then(r => r.json())
        .then(services => {
            const select = document.getElementById('corr-service-select');
            if (select && services && services.length > 0) {
                const currentVal = select.value;
                select.innerHTML = '<option value="">Select Service</option>' +
                    services.map(s => `<option value="${s.name}">${s.display_name || s.name}</option>`).join('');
                if (currentVal) select.value = currentVal;
            }
        })
        .catch(() => {});

    // Also load timeline if service selected
    loadServiceTimeline();
}

function renderCorrelation(corr) {
    const confidence = Math.round((corr.confidence || 0) * 100);
    const confidenceColor = confidence >= 80 ? '#f4212e' : confidence >= 60 ? '#ffd400' : '#71767b';
    const timeDelta = formatDuration(corr.time_delta);

    return `<div style="padding:0.5rem;border-bottom:1px solid #333;cursor:pointer;" onclick="showDeployContext('${corr.deployment?.id}')">
        <div style="display:flex;justify-content:space-between;align-items:center;">
            <span style="font-weight:bold;color:#1d9bf0;">${escapeHtml(corr.deployment?.version || 'Unknown')}</span>
            <span style="font-size:0.75rem;color:${confidenceColor};">${confidence}% confidence</span>
        </div>
        <div style="font-size:0.8rem;color:#888;">
            ${escapeHtml(corr.deployment?.service || '')} • ${timeDelta} before incident
        </div>
        <div style="font-size:0.75rem;color:#71767b;margin-top:0.3rem;">
            ${escapeHtml(corr.reason || '')}
        </div>
    </div>`;
}

function loadServiceTimeline() {
    const service = document.getElementById('corr-service-select')?.value;
    const timeRange = document.getElementById('corr-timerange')?.value || '1h';

    if (!service) {
        const listEl = document.getElementById('corr-timeline-list');
        if (listEl) listEl.innerHTML = '<div class="empty-state" style="padding:1rem;text-align:center;color:#666;">Select a service to view timeline</div>';
        return;
    }

    fetch(`/api/correlate/service/${encodeURIComponent(service)}/timeline?since=${timeRange}`)
        .then(r => r.json())
        .then(data => {
            correlationData.timeline = data;

            const totalEl = document.getElementById('corr-total-events');
            const errorsEl = document.getElementById('corr-error-traces');
            const listEl = document.getElementById('corr-timeline-list');

            if (totalEl && data.summary) totalEl.textContent = data.summary.total_events || 0;
            if (errorsEl && data.summary) errorsEl.textContent = data.summary.error_log_count || 0;

            if (listEl) {
                if (!data.events || data.events.length === 0) {
                    listEl.innerHTML = '<div class="empty-state" style="padding:1rem;text-align:center;color:#666;">No events in this time range</div>';
                } else {
                    listEl.innerHTML = data.events.map(evt => renderTimelineEvent(evt)).join('');
                }
            }
        })
        .catch(err => {
            console.error('Error loading timeline:', err);
            const listEl = document.getElementById('corr-timeline-list');
            if (listEl) listEl.innerHTML = '<div class="empty-state" style="padding:1rem;text-align:center;color:#f4212e;">Error loading timeline</div>';
        });
}

function renderTimelineEvent(evt) {
    const typeColors = {
        'deploy': '#1d9bf0',
        'incident': '#f4212e',
        'trace': '#00ba7c',
        'log': '#71767b',
        'alert': '#ffd400'
    };
    const typeIcons = {
        'deploy': '🚀',
        'incident': '🚨',
        'trace': '🔍',
        'log': '📝',
        'alert': '⚠️'
    };
    const color = typeColors[evt.type] || '#71767b';
    const icon = typeIcons[evt.type] || '•';
    const time = new Date(evt.timestamp).toLocaleTimeString();
    const severityClass = evt.severity === 'error' || evt.severity === 'critical' ? 'color:#f4212e;' :
                         evt.severity === 'warn' || evt.severity === 'high' ? 'color:#ffd400;' : '';

    return `<div style="display:flex;gap:0.5rem;padding:0.4rem;border-bottom:1px solid #2f3336;cursor:pointer;" onclick="showEventDetail('${evt.type}', '${evt.id}')">
        <div style="font-size:0.9rem;">${icon}</div>
        <div style="flex:1;">
            <div style="display:flex;justify-content:space-between;">
                <span style="font-weight:bold;color:${color};font-size:0.8rem;">${evt.type.toUpperCase()}</span>
                <span style="font-size:0.7rem;color:#71767b;">${time}</span>
            </div>
            <div style="font-size:0.8rem;${severityClass}">${escapeHtml(evt.summary || '')}</div>
        </div>
    </div>`;
}

function formatDuration(ns) {
    if (!ns) return 'unknown';
    const ms = ns / 1000000;
    const secs = ms / 1000;
    const mins = secs / 60;
    if (mins >= 1) return Math.round(mins) + 'm';
    if (secs >= 1) return Math.round(secs) + 's';
    return Math.round(ms) + 'ms';
}

function showDeployContext(deployId) {
    if (!deployId) return;

    fetch(`/api/correlate/deploy/${deployId}`)
        .then(r => r.json())
        .then(ctx => {
            const modal = document.createElement('div');
            modal.className = 'modal-overlay';
            modal.onclick = e => { if (e.target === modal) modal.remove(); };

            const deploy = ctx.deployment || {};
            const incidentsHtml = (ctx.following_incidents || []).length > 0
                ? ctx.following_incidents.map(inc => `<div style="padding:0.3rem;border-bottom:1px solid #333;">
                    <span class="severity-badge severity-${inc.severity}">${inc.severity}</span>
                    ${escapeHtml(inc.title)}
                  </div>`).join('')
                : '<span style="color:#71767b">No incidents following this deploy</span>';

            const errComp = ctx.errors_comparison || {};
            const errChange = errComp.change_percent || 0;
            const errColor = errChange > 50 ? '#f4212e' : errChange > 0 ? '#ffd400' : '#00ba7c';

            modal.innerHTML = `
                <div class="modal" style="max-width:600px;">
                    <div class="modal-header">
                        <span class="modal-title">Deploy Context: ${escapeHtml(deploy.version || deploy.id)}</span>
                        <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
                    </div>
                    <div class="modal-body">
                        <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;margin-bottom:1rem;">
                            <div>
                                <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Service</div>
                                <div style="font-size:1rem;">${escapeHtml(deploy.service || 'Unknown')}</div>
                            </div>
                            <div>
                                <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Impact</div>
                                <div style="font-size:1rem;color:${ctx.impact === 'negative' ? '#f4212e' : ctx.impact === 'positive' ? '#00ba7c' : '#71767b'}">${ctx.impact || 'neutral'}</div>
                            </div>
                        </div>
                        <div style="margin-bottom:1rem;">
                            <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Error Rate Change</div>
                            <div style="display:flex;gap:1rem;">
                                <div>Before: ${errComp.errors_before || 0} (${(errComp.rate_before || 0).toFixed(1)}/min)</div>
                                <div>After: ${errComp.errors_after || 0} (${(errComp.rate_after || 0).toFixed(1)}/min)</div>
                                <div style="color:${errColor}">${errChange > 0 ? '+' : ''}${errChange.toFixed(0)}%</div>
                            </div>
                        </div>
                        <div>
                            <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Following Incidents</div>
                            ${incidentsHtml}
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                    </div>
                </div>`;
            document.body.appendChild(modal);
        })
        .catch(err => console.error('Error loading deploy context:', err));
}

function showEventDetail(type, id) {
    if (type === 'trace') {
        // Load trace context
        fetch(`/api/correlate/trace/${id}`)
            .then(r => r.json())
            .then(ctx => {
                showTraceContextModal(ctx);
            })
            .catch(err => console.error('Error loading trace context:', err));
    } else if (type === 'incident') {
        fetch(`/api/correlate/incident/${id}`)
            .then(r => r.json())
            .then(ctx => {
                showIncidentContextModal(ctx);
            })
            .catch(err => console.error('Error loading incident context:', err));
    }
}

function showTraceContextModal(ctx) {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };

    const trace = ctx.trace || {};
    const logsHtml = (ctx.logs || []).length > 0
        ? ctx.logs.slice(0, 20).map(log => {
            const levelColor = log.level === 'error' ? '#f4212e' : log.level === 'warn' ? '#ffd400' : '#71767b';
            return `<div style="padding:0.2rem 0;border-bottom:1px solid #2f3336;font-size:0.75rem;">
                <span style="color:${levelColor};font-weight:bold;">${log.level}</span>
                <span style="color:#71767b;margin-left:0.5rem;">${new Date(log.timestamp).toLocaleTimeString()}</span>
                <div style="color:#e7e9ea;">${escapeHtml(log.message?.substring(0, 200) || '')}</div>
            </div>`;
        }).join('')
        : '<span style="color:#71767b">No logs for this trace</span>';

    modal.innerHTML = `
        <div class="modal" style="max-width:700px;">
            <div class="modal-header">
                <span class="modal-title">Trace Context</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body" style="max-height:70vh;overflow-y:auto;">
                <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:1rem;margin-bottom:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Service</div>
                        <div>${escapeHtml(ctx.service || trace.service_name || 'Unknown')}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Duration</div>
                        <div>${(ctx.duration_ms || trace.duration_ms || 0).toFixed(2)}ms</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Status</div>
                        <div style="color:${ctx.status === 'ERROR' ? '#f4212e' : '#00ba7c'}">${ctx.status || trace.status || 'OK'}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Errors</div>
                        <div style="color:${ctx.error_count > 0 ? '#f4212e' : '#00ba7c'}">${ctx.error_count || 0}</div>
                    </div>
                </div>
                <div style="margin-bottom:1rem;">
                    <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Trace ID</div>
                    <code style="font-size:0.8rem;background:#1a1a1a;padding:0.3rem;border-radius:3px;">${ctx.trace_id || ''}</code>
                </div>
                <div>
                    <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Correlated Logs (${(ctx.logs || []).length})</div>
                    <div style="max-height:300px;overflow-y:auto;background:#1a1a1a;padding:0.5rem;border-radius:4px;">
                        ${logsHtml}
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>`;
    document.body.appendChild(modal);
}

function showIncidentContextModal(ctx) {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = e => { if (e.target === modal) modal.remove(); };

    const inc = ctx.incident || {};
    const cause = ctx.probable_cause;

    const deploysHtml = (ctx.preceding_deploys || []).length > 0
        ? ctx.preceding_deploys.map(d => `<div style="padding:0.3rem;border-bottom:1px solid #333;">
            🚀 ${escapeHtml(d.version)} - ${escapeHtml(d.service)}
            <span style="color:#71767b;font-size:0.75rem;">${new Date(d.timestamp).toLocaleString()}</span>
          </div>`).join('')
        : '<span style="color:#71767b">No recent deploys</span>';

    const timelineHtml = (ctx.timeline || []).slice(0, 15).map(evt => {
        const icon = evt.type === 'deploy' ? '🚀' : evt.type === 'incident' ? '🚨' : evt.type === 'trace' ? '🔍' : '📝';
        return `<div style="padding:0.2rem 0;font-size:0.8rem;">
            ${icon} <span style="color:#71767b;">${new Date(evt.timestamp).toLocaleTimeString()}</span> ${escapeHtml(evt.summary || '')}
        </div>`;
    }).join('');

    modal.innerHTML = `
        <div class="modal" style="max-width:700px;">
            <div class="modal-header">
                <span class="modal-title">Incident Context: ${escapeHtml(inc.title || '')}</span>
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-body" style="max-height:70vh;overflow-y:auto;">
                <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:1rem;margin-bottom:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Severity</div>
                        <div class="severity-badge severity-${inc.severity}">${inc.severity || 'unknown'}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Status</div>
                        <div>${inc.status || 'unknown'}</div>
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;">Service</div>
                        <div>${escapeHtml(inc.service || 'Unknown')}</div>
                    </div>
                </div>
                ${cause ? `
                <div style="background:#2a1a1a;border:1px solid #f4212e;border-radius:4px;padding:0.75rem;margin-bottom:1rem;">
                    <div style="color:#f4212e;font-weight:bold;margin-bottom:0.3rem;">⚠️ Probable Cause Detected</div>
                    <div>Deploy <strong>${escapeHtml(cause.deployment?.version || '')}</strong> occurred ${formatDuration(cause.time_delta)} before incident</div>
                    <div style="color:#71767b;font-size:0.8rem;">${escapeHtml(cause.reason || '')} (${Math.round((cause.confidence || 0) * 100)}% confidence)</div>
                </div>
                ` : ''}
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Preceding Deploys</div>
                        ${deploysHtml}
                    </div>
                    <div>
                        <div style="color:#71767b;font-size:0.7rem;text-transform:uppercase;margin-bottom:0.5rem;">Timeline</div>
                        <div style="max-height:200px;overflow-y:auto;">${timelineHtml || '<span style="color:#71767b">No timeline events</span>'}</div>
                    </div>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
            </div>
        </div>`;
    document.body.appendChild(modal);
}

setTimeout(loadCorrelations, 1500);
setInterval(loadCorrelations, 60000);

// ============ Kubernetes Functions ============
// moved to top

function switchK8sTab(tab) {
    currentK8sTab = tab;
    document.querySelectorAll('.k8s-tab').forEach(t => t.classList.remove('active'));
    document.querySelector(`.k8s-tab[data-tab="${tab}"]`).classList.add('active');
    document.querySelectorAll('.k8s-content').forEach(c => c.classList.remove('active'));
    document.getElementById(`k8s-${tab}-content`).classList.add('active');
}

function loadKubernetes() {
    const nsSelect = document.getElementById('k8s-namespace-select');
    const selectedNs = nsSelect ? nsSelect.value : '';
    const nsParam = selectedNs ? `?namespace=${encodeURIComponent(selectedNs)}` : '';

    // Load summary/cluster info
    fetch('/api/k8s/summary')
        .then(r => r.json())
        .then(summary => {
            if (!summary) return;

            const podsEl = document.getElementById('k8s-pods');
            const deploysEl = document.getElementById('k8s-deployments');
            const servicesEl = document.getElementById('k8s-services');
            const nodesEl = document.getElementById('k8s-nodes');
            const clusterNameEl = document.getElementById('k8s-cluster-name');

            if (podsEl) {
                podsEl.textContent = `${summary.running_pods || 0}/${summary.total_pods || 0}`;
                podsEl.style.color = summary.running_pods === summary.total_pods ? '#00ba7c' : '#ffd400';
            }
            if (deploysEl) {
                deploysEl.textContent = `${summary.ready_deployments || 0}/${summary.total_deployments || 0}`;
                deploysEl.style.color = summary.ready_deployments === summary.total_deployments ? '#00ba7c' : '#ffd400';
            }
            if (servicesEl) servicesEl.textContent = summary.total_services || 0;
            if (nodesEl) {
                nodesEl.textContent = `${summary.ready_nodes || 0}/${summary.total_nodes || 0}`;
                nodesEl.style.color = summary.ready_nodes === summary.total_nodes ? '#00ba7c' : '#ffd400';
            }
            if (clusterNameEl) clusterNameEl.textContent = summary.cluster_name ? `Cluster: ${summary.cluster_name}` : '';
        })
        .catch(() => {});

    // Load namespaces for dropdown
    fetch('/api/k8s/namespaces')
        .then(r => r.json())
        .then(namespaces => {
            if (!namespaces || !nsSelect) return;
            const currentVal = nsSelect.value;
            nsSelect.innerHTML = '<option value="">All Namespaces</option>';
            namespaces.forEach(ns => {
                const opt = document.createElement('option');
                opt.value = ns.name;
                opt.textContent = ns.name;
                nsSelect.appendChild(opt);
            });
            nsSelect.value = currentVal;
        })
        .catch(() => {});

    // Load pods
    fetch(`/api/k8s/pods${nsParam}`)
        .then(r => r.json())
        .then(pods => {
            const el = document.getElementById('k8s-pods-content');
            if (!el) return;
            if (!pods || pods.length === 0) {
                el.innerHTML = '<div class="empty-state">No pods found</div>';
                return;
            }
            el.innerHTML = pods.map(pod => {
                const statusClass = (pod.phase || 'unknown').toLowerCase();
                const readyContainers = pod.containers ? pod.containers.filter(c => c.ready).length : 0;
                const totalContainers = pod.containers ? pod.containers.length : 0;
                const restarts = pod.containers ? pod.containers.reduce((sum, c) => sum + (c.restarts || 0), 0) : 0;

                return `<div class="k8s-item">
                    <div class="k8s-status ${statusClass}"></div>
                    <div class="k8s-info">
                        <div class="k8s-name">
                            ${pod.name}
                            <span class="k8s-namespace">${pod.namespace}</span>
                        </div>
                        <div class="k8s-meta">
                            <span>${pod.phase || 'Unknown'}</span>
                            <span>Ready: ${readyContainers}/${totalContainers}</span>
                            <span>Restarts: ${restarts}</span>
                            ${pod.node_name ? `<span>Node: ${pod.node_name}</span>` : ''}
                        </div>
                    </div>
                    <div class="k8s-metrics">
                        ${pod.ip ? `<div class="k8s-metric"><span class="k8s-metric-value" style="font-size:0.65rem">${pod.ip}</span><span>IP</span></div>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});

    // Load deployments
    fetch(`/api/k8s/deployments${nsParam}`)
        .then(r => r.json())
        .then(deploys => {
            const el = document.getElementById('k8s-deploys-content');
            if (!el) return;
            if (!deploys || deploys.length === 0) {
                el.innerHTML = '<div class="empty-state">No deployments found</div>';
                return;
            }
            el.innerHTML = deploys.map(d => {
                const ready = d.ready_replicas || 0;
                const desired = d.replicas || 0;
                const statusClass = ready === desired ? 'ready' : 'notready';

                return `<div class="k8s-item">
                    <div class="k8s-status ${statusClass}"></div>
                    <div class="k8s-info">
                        <div class="k8s-name">
                            ${d.name}
                            <span class="k8s-namespace">${d.namespace}</span>
                        </div>
                        <div class="k8s-meta">
                            <span>Ready: ${ready}/${desired}</span>
                            <span>Available: ${d.available_replicas || 0}</span>
                            <span>Updated: ${d.updated_replicas || 0}</span>
                        </div>
                    </div>
                    <div class="k8s-metrics">
                        <div class="k8s-metric">
                            <span class="k8s-metric-value">${d.replicas || 0}</span>
                            <span>Replicas</span>
                        </div>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});

    // Load services
    fetch(`/api/k8s/services${nsParam}`)
        .then(r => r.json())
        .then(services => {
            const el = document.getElementById('k8s-services-content');
            if (!el) return;
            if (!services || services.length === 0) {
                el.innerHTML = '<div class="empty-state">No services found</div>';
                return;
            }
            el.innerHTML = services.map(svc => {
                const ports = svc.ports ? svc.ports.map(p => `${p.port}${p.node_port ? ':' + p.node_port : ''}/${p.protocol || 'TCP'}`).join(', ') : '-';

                return `<div class="k8s-item">
                    <div class="k8s-status ready"></div>
                    <div class="k8s-info">
                        <div class="k8s-name">
                            ${svc.name}
                            <span class="k8s-namespace">${svc.namespace}</span>
                        </div>
                        <div class="k8s-meta">
                            <span>${svc.type || 'ClusterIP'}</span>
                            <span>IP: ${svc.cluster_ip || '-'}</span>
                            <span>Ports: ${ports}</span>
                        </div>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});

    // Load nodes
    fetch('/api/k8s/nodes')
        .then(r => r.json())
        .then(nodes => {
            const el = document.getElementById('k8s-nodes-content');
            if (!el) return;
            if (!nodes || nodes.length === 0) {
                el.innerHTML = '<div class="empty-state">No nodes found</div>';
                return;
            }
            el.innerHTML = nodes.map(node => {
                const statusClass = node.ready ? 'ready' : 'notready';
                const roles = node.labels ? Object.keys(node.labels).filter(k => k.startsWith('node-role.kubernetes.io/')).map(k => k.replace('node-role.kubernetes.io/', '')).join(', ') : '-';

                return `<div class="k8s-item">
                    <div class="k8s-status ${statusClass}"></div>
                    <div class="k8s-info">
                        <div class="k8s-name">${node.name}</div>
                        <div class="k8s-meta">
                            <span>${node.ready ? 'Ready' : 'Not Ready'}</span>
                            <span>Roles: ${roles || 'worker'}</span>
                            <span>Version: ${node.kubelet_version || '-'}</span>
                        </div>
                    </div>
                    <div class="k8s-metrics">
                        <div class="k8s-metric">
                            <span class="k8s-metric-value">${node.allocatable_cpu || '-'}</span>
                            <span>CPU</span>
                        </div>
                        <div class="k8s-metric">
                            <span class="k8s-metric-value">${node.allocatable_memory || '-'}</span>
                            <span>Mem</span>
                        </div>
                        <div class="k8s-metric">
                            <span class="k8s-metric-value">${node.allocatable_pods || '-'}</span>
                            <span>Pods</span>
                        </div>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});

    // Load events
    fetch(`/api/k8s/events${nsParam}`)
        .then(r => r.json())
        .then(events => {
            const el = document.getElementById('k8s-events-content');
            if (!el) return;
            if (!events || events.length === 0) {
                el.innerHTML = '<div class="empty-state">No events found</div>';
                return;
            }
            el.innerHTML = events.slice(0, 50).map(evt => {
                const typeClass = (evt.type || 'normal').toLowerCase();
                const timeAgo = evt.last_timestamp ? formatTimeAgo(new Date(evt.last_timestamp)) : '-';

                return `<div class="k8s-event">
                    <span class="k8s-event-type ${typeClass}">${evt.type || 'Normal'}</span>
                    <strong>${evt.reason || '-'}</strong>: ${evt.message || '-'}
                    <span class="k8s-event-time">${timeAgo} - ${evt.involved_object_kind || ''}/${evt.involved_object_name || ''}</span>
                </div>`;
            }).join('');
        })
        .catch(() => {});
}

function formatTimeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    return `${days}d ago`;
}

// Initialize Kubernetes
setTimeout(loadKubernetes, 1200);
setInterval(loadKubernetes, 15000);

// Anomaly Detection functions
function loadAnomalies() {
    // Load stats
    fetch('/api/anomaly/stats')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(stats => {
            // Use demo stats if empty
            if (DemoData.enabled && !stats?.total_anomalies && !stats?.metrics_tracked) {
                stats = DemoData.generateAnomalyStats();
            }
            document.getElementById('anomaly-critical').textContent = stats.critical_count || 0;
            document.getElementById('anomaly-warning').textContent = stats.warning_count || 0;
            document.getElementById('anomaly-total').textContent = stats.total_anomalies || 0;
            document.getElementById('anomaly-metrics').textContent = stats.metrics_tracked || 0;
        })
        .catch(() => {
            if (DemoData.enabled) {
                const stats = DemoData.generateAnomalyStats();
                document.getElementById('anomaly-critical').textContent = stats.critical_count;
                document.getElementById('anomaly-warning').textContent = stats.warning_count;
                document.getElementById('anomaly-total').textContent = stats.total_anomalies;
                document.getElementById('anomaly-metrics').textContent = stats.metrics_tracked;
            }
        });

    // Load recent anomalies
    fetch('/api/anomaly/recent?limit=20')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(anomalies => {
            const list = document.getElementById('anomaly-list');
            if (!list) return;

            // Use demo data if empty and demo mode enabled
            if ((!anomalies || anomalies.length === 0) && DemoData.enabled) {
                anomalies = DemoData.generateAnomalies();
            }
            if (!anomalies || anomalies.length === 0) {
                list.innerHTML = '<div class="empty-state">No anomalies detected. System is healthy!</div>';
                return;
            }

            list.innerHTML = anomalies.map(a => {
                const severity = a.is_critical ? 'critical' : (a.score > 0.7 ? 'warning' : 'info');
                const scoreClass = a.score > 0.8 ? 'high' : (a.score > 0.5 ? 'medium' : 'low');
                const time = new Date(a.timestamp).toLocaleString();
                const relTime = formatRelativeTime(new Date(a.timestamp));

                return `<div class="anomaly-item">
                    <div class="anomaly-severity ${severity}"></div>
                    <div class="anomaly-info">
                        <div class="anomaly-metric">${escapeHtml(a.metric_name || 'Unknown')}</div>
                        <div class="anomaly-meta">
                            <span title="${time}">${relTime}</span>
                            <span>Value: ${a.value?.toFixed(2) || '-'}</span>
                            <span class="anomaly-score ${scoreClass}">Score: ${(a.score * 100).toFixed(0)}%</span>
                        </div>
                        ${a.reason ? `<div style="font-size:0.7rem;color:#71767b;margin-top:0.2rem;">${escapeHtml(a.reason)}</div>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            if (DemoData.enabled) {
                const list = document.getElementById('anomaly-list');
                if (!list) return;
                const anomalies = DemoData.generateAnomalies();
                list.innerHTML = anomalies.map(a => {
                    const severity = a.is_critical ? 'critical' : (a.score > 0.7 ? 'warning' : 'info');
                    const relTime = formatRelativeTime(new Date(a.timestamp));
                    return `<div class="anomaly-item"><div class="anomaly-severity ${severity}"></div><div class="anomaly-info"><div class="anomaly-metric">${escapeHtml(a.metric_name)}</div><div class="anomaly-meta"><span>${relTime}</span><span>Value: ${a.value?.toFixed(2)}</span><span class="anomaly-score">${(a.score * 100).toFixed(0)}%</span></div>${a.reason ? `<div style="font-size:0.7rem;color:#71767b;">${escapeHtml(a.reason)}</div>` : ''}</div></div>`;
                }).join('');
            } else {
                const list = document.getElementById('anomaly-list');
                if (list) list.innerHTML = '<div class="empty-state">Anomaly detection not available</div>';
            }
        });
}

// Alerting functions
function loadAlerts() {
    // Load status
    fetch('/api/alerting/status')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(status => {
            // Use demo stats if empty
            if (DemoData.enabled && !status?.firing_alerts && !status?.total_rules) {
                status = DemoData.generateAlertingStatus();
            }
            document.getElementById('alerts-firing').textContent = status.firing_alerts || 0;
            document.getElementById('alerts-pending').textContent = status.pending_alerts || 0;
            document.getElementById('alerts-silenced').textContent = status.active_silences || 0;
            document.getElementById('alerts-rules').textContent = status.total_rules || 0;
        })
        .catch(() => {
            if (DemoData.enabled) {
                const status = DemoData.generateAlertingStatus();
                document.getElementById('alerts-firing').textContent = status.firing_alerts;
                document.getElementById('alerts-pending').textContent = status.pending_alerts;
                document.getElementById('alerts-silenced').textContent = status.active_silences;
                document.getElementById('alerts-rules').textContent = status.total_rules;
            }
        });

    // Load firing alerts
    fetch('/api/alerting/alerts?state=firing')
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(alerts => {
            const content = document.getElementById('alerting-alerts-content');
            if (!content) return;

            // Use demo data if empty and demo mode enabled
            if ((!alerts || alerts.length === 0) && DemoData.enabled) {
                alerts = DemoData.generateAlerts();
            }
            if (!alerts || alerts.length === 0) {
                content.innerHTML = '<div class="empty-state">No firing alerts. All clear!</div>';
                return;
            }

            content.innerHTML = alerts.map(alert => {
                const time = new Date(alert.starts_at).toLocaleString();
                const relTime = formatRelativeTime(new Date(alert.starts_at));
                const severity = alert.severity || 'warning';
                const labels = Object.entries(alert.labels || {}).map(([k, v]) =>
                    `<span class="alert-label">${escapeHtml(k)}=${escapeHtml(v)}</span>`
                ).join('');

                return `<div class="alert-item" onclick="showAlertDetail('${alert.id}')">
                    <div class="alert-severity ${severity}"></div>
                    <div class="alert-info">
                        <div class="alert-header">
                            <span class="alert-name">${escapeHtml(alert.rule_name || 'Alert')}</span>
                            <span class="alert-state ${alert.state}">${alert.state}</span>
                        </div>
                        <div class="alert-meta">
                            <span title="${time}">${relTime}</span>
                            <span class="alert-value">Value: ${alert.value?.toFixed(2) || '-'}</span>
                        </div>
                        ${labels ? `<div class="alert-labels">${labels}</div>` : ''}
                    </div>
                    <div class="alert-actions" onclick="event.stopPropagation()">
                        <button class="btn" onclick="ackAlert('${alert.id}')" title="Acknowledge">&#10003;</button>
                        <button class="btn" onclick="silenceAlert('${alert.id}')" title="Silence">&#128263;</button>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            if (DemoData.enabled) {
                const content = document.getElementById('alerting-alerts-content');
                if (!content) return;
                const alerts = DemoData.generateAlerts();
                content.innerHTML = alerts.map(alert => {
                    const relTime = formatRelativeTime(new Date(alert.starts_at));
                    const labels = Object.entries(alert.labels || {}).map(([k, v]) => `<span class="alert-label">${escapeHtml(k)}=${escapeHtml(v)}</span>`).join('');
                    return `<div class="alert-item"><div class="alert-severity ${alert.severity || 'warning'}"></div><div class="alert-info"><div class="alert-name">${escapeHtml(alert.name)}</div><div class="alert-summary">${escapeHtml(alert.annotations?.summary || '')}</div><div class="alert-meta"><span>${relTime}</span></div>${labels ? `<div class="alert-labels">${labels}</div>` : ''}</div></div>`;
                }).join('');
            } else {
                const content = document.getElementById('alerting-alerts-content');
                if (content) content.innerHTML = '<div class="empty-state">Alerting not available</div>';
            }
        });

    // Load rules
    fetch('/api/alerting/rules')
        .then(r => r.json())
        .then(rules => {
            const content = document.getElementById('alerting-rules-content');
            if (!content) return;

            if (!rules || rules.length === 0) {
                content.innerHTML = '<div class="empty-state">No alert rules configured</div>';
                return;
            }

            content.innerHTML = rules.map(rule => {
                const typeLabel = rule.type || 'threshold';
                return `<div class="alert-item">
                    <div class="alert-severity ${rule.enabled ? 'info' : 'warning'}" style="opacity:${rule.enabled ? 1 : 0.3}"></div>
                    <div class="alert-info">
                        <div class="alert-header">
                            <span class="alert-name">${escapeHtml(rule.name)}</span>
                            <span class="alert-label">${typeLabel}</span>
                            ${!rule.enabled ? '<span class="alert-state" style="background:#2f3336;color:#71767b">disabled</span>' : ''}
                        </div>
                        <div class="alert-meta">
                            <span>${rule.condition} ${rule.threshold}</span>
                            ${rule.query ? `<span style="font-family:monospace;color:#1d9bf0">${escapeHtml(rule.query.substring(0, 30))}...</span>` : ''}
                        </div>
                    </div>
                    <div class="alert-actions" onclick="event.stopPropagation()">
                        <button class="btn" onclick="toggleRule('${rule.id}', ${!rule.enabled})">${rule.enabled ? 'Disable' : 'Enable'}</button>
                        <button class="btn" style="color:#f4212e" onclick="deleteRule('${rule.id}')">Delete</button>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});

    // Load silences
    fetch('/api/alerting/silences?all=true')
        .then(r => r.json())
        .then(silences => {
            const content = document.getElementById('alerting-silences-content');
            if (!content) return;

            if (!silences || silences.length === 0) {
                content.innerHTML = '<div class="empty-state">No silences configured</div>';
                return;
            }

            content.innerHTML = silences.map(s => {
                const matchers = (s.matchers || []).map(m => `${m.name}${m.is_equal ? '=' : '!='}${m.value}`).join(', ');
                const endsAt = new Date(s.ends_at).toLocaleString();
                const state = s.state || 'active';

                return `<div class="silence-item">
                    <div class="silence-info">
                        <span class="silence-matchers">${escapeHtml(matchers) || 'All alerts'}</span>
                        <span class="silence-state ${state}">${state}</span>
                    </div>
                    <div class="silence-meta">
                        <span>Ends: ${endsAt}</span>
                        ${s.comment ? `<span>${escapeHtml(s.comment)}</span>` : ''}
                        ${s.created_by ? `<span>by ${escapeHtml(s.created_by)}</span>` : ''}
                        ${state === 'active' ? `<button class="btn" style="margin-left:auto" onclick="expireSilence('${s.id}')">Expire</button>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});
}

function switchAlertingTab(tab) {
    document.querySelectorAll('.alerting-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.alerting-content').forEach(c => c.classList.remove('active'));
    document.querySelector(`.alerting-tab[data-tab="${tab}"]`)?.classList.add('active');
    document.getElementById(`alerting-${tab}-content`)?.classList.add('active');
}

function ackAlert(alertId) {
    fetch(`/api/alerting/alerts/${alertId}/acknowledge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: 'dashboard' })
    }).then(() => loadAlerts());
}

function silenceAlert(alertId) {
    const duration = prompt('Silence duration (e.g. 1h, 30m, 2h):', '1h');
    if (!duration) return;
    fetch(`/api/alerting/alerts/${alertId}/silence`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ duration, created_by: 'dashboard', comment: 'Silenced from dashboard' })
    }).then(() => loadAlerts());
}

function expireSilence(silenceId) {
    fetch(`/api/alerting/silences/${silenceId}/expire`, { method: 'POST' })
        .then(() => loadAlerts());
}

function toggleRule(ruleId, enabled) {
    fetch(`/api/alerting/rules/${ruleId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled })
    }).then(() => loadAlerts());
}

function deleteRule(ruleId) {
    if (!confirm('Delete this alert rule?')) return;
    fetch(`/api/alerting/rules/${ruleId}`, { method: 'DELETE' })
        .then(() => loadAlerts());
}

function showAlertDetail(alertId) {
    fetch(`/api/alerting/alerts/${alertId}`)
        .then(r => r.json())
        .then(alert => {
            const modal = document.createElement('div');
            modal.className = 'modal-overlay';
            modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
            modal.innerHTML = `
                <div class="modal">
                    <div class="modal-header">
                        <span class="modal-title">${escapeHtml(alert.rule_name || 'Alert')}</span>
                        <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <p><strong>State:</strong> ${alert.state}</p>
                        <p><strong>Severity:</strong> ${alert.severity}</p>
                        <p><strong>Value:</strong> ${alert.value}</p>
                        <p><strong>Threshold:</strong> ${alert.threshold}</p>
                        <p><strong>Started:</strong> ${new Date(alert.starts_at).toLocaleString()}</p>
                        <p><strong>Labels:</strong></p>
                        <pre style="background:#2f3336;padding:0.5rem;border-radius:4px;font-size:0.75rem;overflow:auto">${JSON.stringify(alert.labels, null, 2)}</pre>
                    </div>
                    <div class="modal-footer">
                        <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
        });
}

function showNewAlertRuleModal() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Alert Rule</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Name *</label>
                    <input type="text" id="rule-name" class="form-input" placeholder="High CPU Usage">
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Type</label>
                        <select id="rule-type" class="form-select">
                            <option value="threshold">Threshold</option>
                            <option value="anomaly">Anomaly</option>
                            <option value="absence">Absence</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Severity</label>
                        <select id="rule-severity" class="form-select">
                            <option value="critical">Critical</option>
                            <option value="warning" selected>Warning</option>
                            <option value="info">Info</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-label">Metric or Query</label>
                    <input type="text" id="rule-metric" class="form-input" placeholder="cpu_usage or WatchQL query">
                </div>
                <div class="form-row">
                    <div class="form-group">
                        <label class="form-label">Condition</label>
                        <select id="rule-condition" class="form-select">
                            <option value="gt">&gt; Greater than</option>
                            <option value="gte">&gt;= Greater or equal</option>
                            <option value="lt">&lt; Less than</option>
                            <option value="lte">&lt;= Less or equal</option>
                            <option value="eq">= Equal</option>
                            <option value="neq">!= Not equal</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label class="form-label">Threshold</label>
                        <input type="number" id="rule-threshold" class="form-input" placeholder="80">
                    </div>
                </div>
                <div class="form-group">
                    <label class="form-label">For Duration (e.g. 5m, 1h)</label>
                    <input type="text" id="rule-duration" class="form-input" placeholder="0s (fire immediately)">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createAlertRule()">Create Rule</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createAlertRule() {
    const name = document.getElementById('rule-name').value;
    const type = document.getElementById('rule-type').value;
    const severity = document.getElementById('rule-severity').value;
    const metric = document.getElementById('rule-metric').value;
    const condition = document.getElementById('rule-condition').value;
    const threshold = parseFloat(document.getElementById('rule-threshold').value) || 0;
    const duration = document.getElementById('rule-duration').value || '0s';

    if (!name) { alert('Name is required'); return; }

    fetch('/api/alerting/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            name,
            type,
            enabled: true,
            metric: metric,
            condition,
            threshold,
            labels: { severity },
            for_duration: duration
        })
    })
    .then(r => { if (!r.ok) throw new Error('Failed'); return r.json(); })
    .then(() => {
        document.querySelector('.modal-overlay')?.remove();
        loadAlerts();
    })
    .catch(() => alert('Failed to create rule'));
}

function showNewSilenceModal() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create Silence</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Matchers (label=value, one per line)</label>
                    <textarea id="silence-matchers" class="form-input" rows="3" placeholder="severity=warning&#10;team=platform"></textarea>
                </div>
                <div class="form-group">
                    <label class="form-label">Duration</label>
                    <select id="silence-duration" class="form-select">
                        <option value="1h">1 hour</option>
                        <option value="2h">2 hours</option>
                        <option value="4h">4 hours</option>
                        <option value="8h">8 hours</option>
                        <option value="24h">24 hours</option>
                        <option value="7d">7 days</option>
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label">Comment</label>
                    <input type="text" id="silence-comment" class="form-input" placeholder="Maintenance window">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createSilence()">Create Silence</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createSilence() {
    const matchersText = document.getElementById('silence-matchers').value;
    const duration = document.getElementById('silence-duration').value;
    const comment = document.getElementById('silence-comment').value;

    const matchers = matchersText.split('\n').filter(l => l.trim()).map(line => {
        const [name, value] = line.split('=');
        return { name: name?.trim(), value: value?.trim(), is_equal: true, is_regex: false };
    }).filter(m => m.name && m.value);

    const durationMs = {
        '1h': 3600000, '2h': 7200000, '4h': 14400000,
        '8h': 28800000, '24h': 86400000, '7d': 604800000
    }[duration] || 3600000;

    fetch('/api/alerting/silences', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            matchers,
            starts_at: new Date().toISOString(),
            ends_at: new Date(Date.now() + durationMs).toISOString(),
            created_by: 'dashboard',
            comment
        })
    })
    .then(r => { if (!r.ok) throw new Error('Failed'); return r.json(); })
    .then(() => {
        document.querySelector('.modal-overlay')?.remove();
        loadAlerts();
    })
    .catch(() => alert('Failed to create silence'));
}

// Notifications widget functions
function loadNotifyWidget() {
    // Load channels
    fetch('/api/notify/channels')
        .then(r => r.json())
        .then(channels => {
            const enabledCount = channels.filter(c => c.enabled).length;
            document.getElementById('notify-channels').textContent = channels.length;
            document.getElementById('notify-enabled').textContent = enabledCount;

            const content = document.getElementById('notify-channels-content');
            if (!content) return;

            if (!channels || channels.length === 0) {
                content.innerHTML = '<div class="empty-state">No notification channels configured. <a href="#" onclick="showNotificationChannels();return false" style="color:#1d9bf0">Add one</a></div>';
                return;
            }

            const typeIcons = {
                webhook: '&#128279;', slack: '&#128172;', email: '&#9993;',
                pagerduty: '&#128221;', opsgenie: '&#128276;', msteams: '&#128187;', discord: '&#127918;'
            };

            content.innerHTML = channels.map(ch => {
                const icon = typeIcons[ch.type] || '&#128276;';
                return `<div class="notify-channel" onclick="showNotificationChannels()">
                    <div class="notify-channel-icon ${ch.type}">${icon}</div>
                    <div class="notify-channel-info">
                        <div class="notify-channel-name">
                            <span class="${ch.enabled ? 'enabled' : 'disabled'}"></span>
                            ${escapeHtml(ch.name)}
                        </div>
                        <div class="notify-channel-meta">${ch.type}${ch.enabled ? '' : ' (disabled)'}</div>
                    </div>
                    <div class="notify-channel-stats">
                        <span class="success">&#10003; ${ch.success_count || 0}</span>
                        <span class="failed">&#10007; ${ch.failure_count || 0}</span>
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            const content = document.getElementById('notify-channels-content');
            if (content) content.innerHTML = '<div class="empty-state">Notifications not available</div>';
        });

    // Load history
    fetch('/api/notify/history')
        .then(r => r.json())
        .then(logs => {
            // Count stats from logs
            const now = Date.now();
            const day = 24 * 60 * 60 * 1000;
            const recent = logs.filter(l => new Date(l.sent_at).getTime() > now - day);
            const sent = recent.filter(l => l.success).length;
            const failed = recent.filter(l => !l.success).length;

            document.getElementById('notify-sent').textContent = sent;
            document.getElementById('notify-failed').textContent = failed;

            const content = document.getElementById('notify-history-content');
            if (!content) return;

            if (!logs || logs.length === 0) {
                content.innerHTML = '<div class="empty-state">No notifications sent yet</div>';
                return;
            }

            content.innerHTML = logs.slice(0, 50).map(log => {
                const time = formatRelativeTime(new Date(log.sent_at));
                return `<div class="notify-history-item">
                    <div class="notify-history-header">
                        <span class="notify-history-title">${escapeHtml(log.notification_type || 'alert')}</span>
                        <span class="notify-history-status ${log.success ? 'success' : 'failed'}">${log.success ? 'Sent' : 'Failed'}</span>
                    </div>
                    <div class="notify-history-meta">
                        <span>${escapeHtml(log.channel_name || 'Unknown channel')}</span>
                        <span>${time}</span>
                        ${log.error ? `<span style="color:#f4212e">${escapeHtml(log.error.substring(0, 50))}</span>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {});
}

function switchNotifyTab(tab) {
    document.querySelectorAll('.notify-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.notify-content').forEach(c => c.classList.remove('active'));
    document.querySelector(`.notify-tab[data-tab="${tab}"]`)?.classList.add('active');
    document.getElementById(`notify-${tab}-content`)?.classList.add('active');
}

// On-Call widget functions
function loadOnCallWidget() {
    // Load schedules
    fetch('/api/oncall/schedules')
        .then(r => r.json())
        .then(schedules => {
            // Use demo data if empty and demo mode enabled
            if ((!schedules || schedules.length === 0) && DemoData.enabled) {
                schedules = DemoData.generateOnCallSchedules();
            }

            document.getElementById('oncall-schedules-count').textContent = schedules?.length || 0;

            const list = document.getElementById('oncall-schedules-list');
            if (!list) return;

            if (!schedules || schedules.length === 0) {
                list.innerHTML = '<div class="empty-state">No on-call schedules. <a href="#" onclick="showNewScheduleModal();return false" style="color:#1d9bf0">Create one</a></div>';
                document.getElementById('oncall-current-person').innerHTML = '<div style="color:#71767b;font-size:0.8rem">No schedules configured</div>';
                return;
            }

            // For each schedule, get current on-call
            let activeCount = 0;
            const currentPeople = [];

            list.innerHTML = schedules.map(sched => {
                const isActive = sched.users && sched.users.length > 0;
                if (isActive) activeCount++;

                const rotationUsers = (sched.users || []).slice(0, 5).map((u, i) =>
                    `<span class="oncall-rotation-user ${i === 0 ? 'current' : ''}">${escapeHtml(u.name || u.email || u.user_id)}</span>`
                ).join('');

                if (sched.users && sched.users.length > 0) {
                    currentPeople.push(sched.users[0]);
                }

                return `<div class="oncall-schedule-item" onclick="showScheduleDetail('${sched.id}')">
                    <div class="oncall-schedule-status ${isActive ? 'active' : 'inactive'}"></div>
                    <div class="oncall-schedule-info">
                        <div class="oncall-schedule-name">${escapeHtml(sched.name)}</div>
                        <div class="oncall-schedule-meta">
                            ${sched.rotation_type || 'weekly'} rotation
                            ${sched.users?.length ? ` • ${sched.users.length} users` : ''}
                        </div>
                        ${rotationUsers ? `<div class="oncall-rotation">${rotationUsers}</div>` : ''}
                    </div>
                </div>`;
            }).join('');

            document.getElementById('oncall-active-count').textContent = activeCount;

            // Update current on-call display
            const currentEl = document.getElementById('oncall-current-person');
            if (currentPeople.length > 0) {
                const person = currentPeople[0];
                const initials = (person.name || person.email || 'U').substring(0, 2).toUpperCase();
                currentEl.innerHTML = `
                    <div class="oncall-avatar">${initials}</div>
                    <div class="oncall-person-info">
                        <div class="oncall-person-name">${escapeHtml(person.name || person.email || person.user_id)}</div>
                        <div class="oncall-person-meta">${person.email || ''}</div>
                    </div>
                    <button class="btn btn-primary" onclick="event.stopPropagation();escalateToOnCall()">Escalate</button>
                `;
            } else {
                currentEl.innerHTML = '<div style="color:#71767b;font-size:0.8rem">No one currently on-call</div>';
            }
        })
        .catch(() => {
            const list = document.getElementById('oncall-schedules-list');
            if (list) list.innerHTML = '<div class="empty-state">On-call service not available</div>';
        });

    // Load policies count
    fetch('/api/oncall/policies')
        .then(r => r.json())
        .then(policies => {
            document.getElementById('oncall-policies-count').textContent = policies?.length || 0;
        })
        .catch(() => {});
}

function showScheduleDetail(schedId) {
    fetch(`/api/oncall/schedules/${schedId}`)
        .then(r => r.json())
        .then(sched => {
            const modal = document.createElement('div');
            modal.className = 'modal-overlay';
            modal.onclick = (e) => { if (e.target === modal) modal.remove(); };

            const users = (sched.users || []).map(u =>
                `<div style="padding:0.3rem 0;border-bottom:1px solid #2f3336">${escapeHtml(u.name || u.email || u.user_id)}</div>`
            ).join('') || '<div style="color:#71767b">No users in rotation</div>';

            modal.innerHTML = `
                <div class="modal">
                    <div class="modal-header">
                        <span class="modal-title">${escapeHtml(sched.name)}</span>
                        <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <p><strong>Rotation:</strong> ${sched.rotation_type || 'weekly'}</p>
                        <p><strong>Timezone:</strong> ${sched.timezone || 'UTC'}</p>
                        <p><strong>Users in rotation:</strong></p>
                        <div style="margin-top:0.5rem">${users}</div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn" style="color:#f4212e" onclick="deleteSchedule('${sched.id}')">Delete</button>
                        <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
        });
}

function showNewScheduleModal() {
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
    modal.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <span class="modal-title">Create On-Call Schedule</span>
                <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label class="form-label">Name *</label>
                    <input type="text" id="sched-name" class="form-input" placeholder="Primary On-Call">
                </div>
                <div class="form-group">
                    <label class="form-label">Rotation Type</label>
                    <select id="sched-rotation" class="form-select">
                        <option value="weekly">Weekly</option>
                        <option value="daily">Daily</option>
                        <option value="custom">Custom</option>
                    </select>
                </div>
                <div class="form-group">
                    <label class="form-label">Users (emails, one per line)</label>
                    <textarea id="sched-users" class="form-input" rows="4" placeholder="alice@example.com&#10;bob@example.com"></textarea>
                </div>
                <div class="form-group">
                    <label class="form-label">Timezone</label>
                    <select id="sched-tz" class="form-select">
                        <option value="UTC">UTC</option>
                        <option value="America/New_York">America/New_York</option>
                        <option value="America/Los_Angeles">America/Los_Angeles</option>
                        <option value="Europe/London">Europe/London</option>
                        <option value="Europe/Berlin">Europe/Berlin</option>
                        <option value="Asia/Tokyo">Asia/Tokyo</option>
                    </select>
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn" onclick="this.closest('.modal-overlay').remove()">Cancel</button>
                <button class="btn btn-primary" onclick="createSchedule()">Create</button>
            </div>
        </div>
    `;
    document.body.appendChild(modal);
}

function createSchedule() {
    const name = document.getElementById('sched-name').value;
    const rotation = document.getElementById('sched-rotation').value;
    const usersText = document.getElementById('sched-users').value;
    const timezone = document.getElementById('sched-tz').value;

    if (!name) { alert('Name is required'); return; }

    const users = usersText.split('\n').filter(l => l.trim()).map(email => ({
        user_id: email.trim(),
        email: email.trim(),
        name: email.trim().split('@')[0]
    }));

    fetch('/api/oncall/schedules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, rotation_type: rotation, users, timezone })
    })
    .then(r => { if (!r.ok) throw new Error('Failed'); return r.json(); })
    .then(() => {
        document.querySelector('.modal-overlay')?.remove();
        loadOnCallWidget();
    })
    .catch(() => alert('Failed to create schedule'));
}

function deleteSchedule(schedId) {
    if (!confirm('Delete this schedule?')) return;
    fetch(`/api/oncall/schedules/${schedId}`, { method: 'DELETE' })
        .then(() => {
            document.querySelector('.modal-overlay')?.remove();
            loadOnCallWidget();
        });
}

function escalateToOnCall() {
    const reason = prompt('Escalation reason:', 'Manual escalation from dashboard');
    if (!reason) return;

    fetch('/api/oncall/escalate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason, source: 'dashboard' })
    })
    .then(r => { if (!r.ok) throw new Error('Failed'); return r.json(); })
    .then(() => alert('Escalation triggered'))
    .catch(() => alert('Failed to escalate'));
}

// Audit log widget functions
function loadAuditWidget() {
    // Load stats
    fetch('/api/audit/stats')
        .then(r => r.json())
        .then(stats => {
            document.getElementById('audit-total').textContent = stats.total_logs || 0;
            document.getElementById('audit-today').textContent = stats.logs_today || 0;
            document.getElementById('audit-failures').textContent = stats.recent_failures || 0;
        })
        .catch(() => {});

    // Get filter values
    const actionFilter = document.getElementById('audit-action-filter')?.value || '';
    const resourceFilter = document.getElementById('audit-resource-filter')?.value || '';

    let url = '/api/audit/logs?limit=50';
    if (actionFilter) url += `&action=${actionFilter}`;
    if (resourceFilter) url += `&resource_type=${resourceFilter}`;

    // Load logs
    fetch(url)
        .then(r => r.json())
        .then(logs => {
            const list = document.getElementById('audit-list');
            if (!list) return;

            if (!logs || logs.length === 0) {
                list.innerHTML = '<div class="empty-state">No audit logs found</div>';
                return;
            }

            list.innerHTML = logs.map(log => {
                const time = formatRelativeTime(new Date(log.timestamp));
                const fullTime = new Date(log.timestamp).toLocaleString();
                const actionClass = log.action || 'read';

                return `<div class="audit-item" onclick="showAuditDetail('${log.id}')">
                    <div class="audit-item-header">
                        <div class="audit-item-action">
                            <span class="action-badge ${actionClass}">${log.action}</span>
                            <span class="audit-item-resource">${log.resource_type}${log.resource_name ? ': ' + escapeHtml(log.resource_name) : ''}</span>
                        </div>
                        <span class="audit-item-outcome ${log.outcome}">${log.outcome}</span>
                    </div>
                    <div class="audit-item-meta">
                        <span title="${fullTime}">${time}</span>
                        ${log.user_email ? `<span>${escapeHtml(log.user_email)}</span>` : ''}
                        ${log.user_ip ? `<span>${log.user_ip}</span>` : ''}
                        ${log.error_message ? `<span style="color:#f4212e">${escapeHtml(log.error_message.substring(0, 50))}</span>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            const list = document.getElementById('audit-list');
            if (list) list.innerHTML = '<div class="empty-state">Audit logging not available (admin only)</div>';
        });
}

function showAuditDetail(logId) {
    fetch(`/api/audit/logs?limit=1`)
        .then(r => r.json())
        .then(logs => {
            // Find the specific log
            const log = logs.find(l => l.id === logId) || logs[0];
            if (!log) return;

            const modal = document.createElement('div');
            modal.className = 'modal-overlay';
            modal.onclick = (e) => { if (e.target === modal) modal.remove(); };

            let changesHtml = '';
            if (log.changes) {
                changesHtml = `
                    <p><strong>Changes:</strong></p>
                    <pre style="background:#2f3336;padding:0.5rem;border-radius:4px;font-size:0.7rem;overflow:auto;max-height:200px">${JSON.stringify(log.changes, null, 2)}</pre>
                `;
            }

            let detailsHtml = '';
            if (log.details) {
                detailsHtml = `
                    <p><strong>Details:</strong></p>
                    <pre style="background:#2f3336;padding:0.5rem;border-radius:4px;font-size:0.7rem;overflow:auto;max-height:200px">${JSON.stringify(log.details, null, 2)}</pre>
                `;
            }

            modal.innerHTML = `
                <div class="modal">
                    <div class="modal-header">
                        <span class="modal-title">Audit Log Details</span>
                        <button class="modal-close" onclick="this.closest('.modal-overlay').remove()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <p><strong>Action:</strong> ${log.action}</p>
                        <p><strong>Resource:</strong> ${log.resource_type} ${log.resource_id ? '(' + log.resource_id + ')' : ''}</p>
                        <p><strong>Outcome:</strong> ${log.outcome}</p>
                        <p><strong>Time:</strong> ${new Date(log.timestamp).toLocaleString()}</p>
                        <p><strong>User:</strong> ${log.user_email || 'Unknown'}</p>
                        <p><strong>IP Address:</strong> ${log.user_ip || 'Unknown'}</p>
                        ${log.error_message ? `<p><strong>Error:</strong> <span style="color:#f4212e">${escapeHtml(log.error_message)}</span></p>` : ''}
                        ${changesHtml}
                        ${detailsHtml}
                    </div>
                    <div class="modal-footer">
                        <button class="btn" onclick="this.closest('.modal-overlay').remove()">Close</button>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);
        });
}

function exportAuditLogs() {
    const start = new Date();
    start.setDate(start.getDate() - 30); // Last 30 days
    const url = `/api/audit/export?start=${start.toISOString()}`;

    window.open(url, '_blank');
}

// Cost Intelligence functions
function loadCostIntel() {
    const content = document.getElementById('cost-intel-content');
    if (!content) return;

    content.innerHTML = '<div class="empty-state">Loading cost estimates...</div>';

    Promise.all([
        fetch('/api/cost/estimate').then(r => r.ok ? r.json() : null),
        fetch('/api/cost/usage').then(r => r.ok ? r.json() : null)
    ]).then(([comparison, usage]) => {
        if (!comparison || !comparison.estimates) {
            content.innerHTML = '<div class="empty-state">Cost data not available yet. Start collecting metrics to see estimates.</div>';
            return;
        }

        const estimates = comparison.estimates;
        const vendors = ['Datadog', 'New Relic', 'Splunk'];
        const savings = comparison.dogwatch_savings || {};

        let vendorCards = vendors.map(v => {
            const est = estimates[v];
            if (!est) return '';
            const savingsPct = savings[v] ? Math.round(savings[v]) : 100;
            const breakdown = est.breakdown || {};

            return `<div class="cost-card ${v === 'Datadog' ? 'highlight' : ''}">
                ${savingsPct > 0 ? `<span class="cost-savings-badge">Save ${savingsPct}%</span>` : ''}
                <div class="cost-vendor">
                    <span>${v}</span>
                </div>
                <div class="cost-amount">$${formatNumber(est.total_monthly)}</div>
                <div class="cost-period">/month</div>
                <div class="cost-breakdown">
                    ${Object.entries(breakdown).slice(0, 4).map(([k, val]) =>
                        `<div class="cost-breakdown-item">
                            <span class="cost-breakdown-label">${k}</span>
                            <span class="cost-breakdown-value">$${formatNumber(val)}</span>
                        </div>`
                    ).join('')}
                </div>
            </div>`;
        }).join('');

        // Usage summary
        const usageSummary = usage ? `
            <div class="cost-summary">
                <div class="cost-summary-stat">
                    <span class="cost-summary-value">${usage.host_count || 0}</span>
                    <span class="cost-summary-label">Hosts</span>
                </div>
                <div class="cost-summary-stat">
                    <span class="cost-summary-value">${usage.container_count || 0}</span>
                    <span class="cost-summary-label">Containers</span>
                </div>
                <div class="cost-summary-stat">
                    <span class="cost-summary-value">${usage.custom_metrics_count || 0}</span>
                    <span class="cost-summary-label">Custom Metrics</span>
                </div>
                <div class="cost-summary-stat">
                    <span class="cost-summary-value">${formatBytes(usage.logs_gb_per_month * 1024 * 1024 * 1024)}</span>
                    <span class="cost-summary-label">Logs/mo</span>
                </div>
                <div class="cost-summary-stat">
                    <span class="cost-summary-value">${formatNumber(usage.spans_per_month || 0)}</span>
                    <span class="cost-summary-label">Spans/mo</span>
                </div>
            </div>
        ` : '';

        content.innerHTML = `
            <div class="cost-grid">${vendorCards}</div>
            ${usageSummary}
            <div style="text-align:center;margin-top:0.75rem;font-size:0.7rem;color:#71767b;">
                With dogwatch: <span style="color:#00ba7c;font-weight:600;">$0/month</span> (self-hosted)
            </div>
        `;
    }).catch(() => {
        content.innerHTML = '<div class="empty-state">Failed to load cost data</div>';
    });
}

function formatNumber(n) {
    if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
    return n?.toFixed(0) || '0';
}

// DB Watch functions
// moved to top

function switchDBWatchTab(tab) {
    dbwatchCurrentTab = tab;
    document.querySelectorAll('.dbwatch-tab').forEach(t => {
        t.classList.toggle('active', t.dataset.tab === tab);
    });
    loadDBWatch();
}

function loadDBWatch() {
    const content = document.getElementById('dbwatch-content');
    if (!content) return;

    const dbFilter = document.getElementById('dbwatch-db-filter')?.value || '';
    const dbParam = dbFilter ? `&db_type=${dbFilter}` : '';

    if (dbwatchCurrentTab === 'stats') {
        loadDBWatchStats(content, dbParam);
    } else if (dbwatchCurrentTab === 'slow') {
        loadDBWatchQueries(content, '/api/dbwatch/slow', dbParam);
    } else {
        loadDBWatchQueries(content, '/api/dbwatch/queries', dbParam);
    }
}

function loadDBWatchQueries(content, endpoint, dbParam) {
    fetch(`${endpoint}?limit=50&since=1h${dbParam}`)
        .then(r => r.ok ? r.json() : Promise.reject('API error'))
        .then(queries => {
            if (!queries || queries.length === 0) {
                content.innerHTML = '<div class="empty-state">No database queries captured yet. Start your application to see queries.</div>';
                return;
            }

            content.innerHTML = queries.map(q => {
                const dbType = (q.db_type || 'unknown').toLowerCase();
                const latencyMs = q.latency_ms || 0;
                const latencyClass = latencyMs > 100 ? 'slow' : 'fast';
                const opType = q.operation || 'QUERY';
                const queryText = q.query || q.command || '-';
                const time = new Date(q.timestamp).toLocaleTimeString();

                return `<div class="dbwatch-query">
                    <div class="dbwatch-query-header">
                        <div class="dbwatch-query-type">
                            <span class="dbwatch-db-badge ${dbType}">${dbType}</span>
                            <span class="dbwatch-op-badge">${opType}</span>
                        </div>
                        <span class="dbwatch-query-latency ${latencyClass}">${latencyMs.toFixed(2)}ms</span>
                    </div>
                    <div class="dbwatch-query-text" title="${escapeHtml(queryText)}">${escapeHtml(queryText.substring(0, 200))}</div>
                    <div class="dbwatch-query-meta">
                        <span>${time}</span>
                        ${q.table ? `<span>Table: ${escapeHtml(q.table)}</span>` : ''}
                        ${q.rows_affected ? `<span>Rows: ${q.rows_affected}</span>` : ''}
                    </div>
                </div>`;
            }).join('');
        })
        .catch(() => {
            content.innerHTML = '<div class="empty-state">Database watch not available</div>';
        });
}

function loadDBWatchStats(content, dbParam) {
    Promise.all([
        fetch(`/api/dbwatch/stats?since=1h${dbParam}`).then(r => r.ok ? r.json() : null),
        fetch(`/api/dbwatch/operations?since=1h${dbParam}`).then(r => r.ok ? r.json() : null)
    ]).then(([stats, operations]) => {
        if (!stats || stats.length === 0) {
            content.innerHTML = '<div class="empty-state">No database statistics available</div>';
            return;
        }

        const statsHtml = stats.map(s => `
            <div class="dbwatch-stats" style="margin:0.5rem;">
                <div class="dbwatch-stat">
                    <span class="dbwatch-stat-value">${s.db_type || 'Unknown'}</span>
                    <span class="dbwatch-stat-label">Database</span>
                </div>
                <div class="dbwatch-stat">
                    <span class="dbwatch-stat-value">${s.total_queries || 0}</span>
                    <span class="dbwatch-stat-label">Total Queries</span>
                </div>
                <div class="dbwatch-stat">
                    <span class="dbwatch-stat-value ${s.avg_latency_ms > 50 ? 'slow' : ''}">${(s.avg_latency_ms || 0).toFixed(1)}ms</span>
                    <span class="dbwatch-stat-label">Avg Latency</span>
                </div>
                <div class="dbwatch-stat">
                    <span class="dbwatch-stat-value slow">${s.slow_queries || 0}</span>
                    <span class="dbwatch-stat-label">Slow Queries</span>
                </div>
                <div class="dbwatch-stat">
                    <span class="dbwatch-stat-value error">${s.errors || 0}</span>
                    <span class="dbwatch-stat-label">Errors</span>
                </div>
            </div>
        `).join('');

        let opsHtml = '';
        if (operations && operations.length > 0) {
            opsHtml = `<div style="padding:0.5rem;">
                <div style="font-size:0.7rem;color:#71767b;margin-bottom:0.5rem;">Operation Breakdown</div>
                ${operations.map(op => `
                    <div style="display:flex;justify-content:space-between;padding:0.3rem 0;font-size:0.75rem;">
                        <span>${op.operation}</span>
                        <span style="font-family:monospace;">${op.count}</span>
                    </div>
                `).join('')}
            </div>`;
        }

        content.innerHTML = statsHtml + opsHtml;
    }).catch(() => {
        content.innerHTML = '<div class="empty-state">Database statistics not available</div>';
    });
}

// Cardinality functions
function loadCardinality() {
    const content = document.getElementById('cardinality-content');
    if (!content) return;

    Promise.all([
        fetch('/api/cardinality/stats').then(r => r.ok ? r.json() : null),
        fetch('/api/cardinality/high?threshold=500').then(r => r.ok ? r.json() : null)
    ]).then(([stats, highCard]) => {
        if (!stats) {
            content.innerHTML = '<div class="empty-state">Cardinality data not available yet. Start collecting metrics to analyze cardinality.</div>';
            return;
        }

        const highCardMetrics = highCard?.metrics || [];
        const highCardCount = highCard?.count || 0;

        const summaryHtml = `
            <div class="cardinality-summary">
                <div class="cardinality-stat">
                    <span class="cardinality-stat-value">${stats.total_metrics || 0}</span>
                    <span class="cardinality-stat-label">Total Metrics</span>
                </div>
                <div class="cardinality-stat">
                    <span class="cardinality-stat-value">${stats.total_series || 0}</span>
                    <span class="cardinality-stat-label">Total Series</span>
                </div>
                <div class="cardinality-stat">
                    <span class="cardinality-stat-value ${highCardCount > 0 ? 'warning' : ''}">${highCardCount}</span>
                    <span class="cardinality-stat-label">High Cardinality</span>
                </div>
                <div class="cardinality-stat">
                    <span class="cardinality-stat-value">${stats.total_tag_keys || 0}</span>
                    <span class="cardinality-stat-label">Tag Keys</span>
                </div>
            </div>
        `;

        let metricsHtml = '';
        if (highCardMetrics.length > 0) {
            const maxSeries = Math.max(...highCardMetrics.map(m => m.series_count || 0), 1);
            metricsHtml = `<div style="font-size:0.7rem;color:#71767b;margin:0.5rem 0;">High Cardinality Metrics (>${highCard.threshold || 500} series)</div>` +
                highCardMetrics.slice(0, 10).map(m => {
                    const pct = ((m.series_count || 0) / maxSeries) * 100;
                    const barClass = m.series_count > 5000 ? 'danger' : m.series_count > 1000 ? 'warning' : '';
                    return `<div class="cardinality-metric">
                        <div style="flex:1;min-width:0;">
                            <div style="display:flex;justify-content:space-between;align-items:center;">
                                <span class="cardinality-metric-name">${escapeHtml(m.name || m.metric_name)}</span>
                                <span class="cardinality-metric-series ${m.series_count > 5000 ? 'danger' : m.series_count > 1000 ? 'warning' : ''}">${m.series_count}</span>
                            </div>
                            <div class="cardinality-bar"><div class="cardinality-bar-fill ${barClass}" style="width:${pct}%"></div></div>
                        </div>
                    </div>`;
                }).join('');
        } else {
            metricsHtml = '<div style="padding:0.5rem;font-size:0.75rem;color:#71767b;">No high cardinality metrics detected. Looking good!</div>';
        }

        content.innerHTML = summaryHtml + metricsHtml;
    }).catch(() => {
        content.innerHTML = '<div class="empty-state">Cardinality explorer not available</div>';
    });
}

// Initialize Anomaly, Alerting, Notifications, On-Call, and Audit
// WebSocket handles real-time updates, polling is fallback only
setTimeout(loadAnomalies, 1300);
setInterval(loadAnomalies, 30000);     // WebSocket handles new anomalies
setTimeout(loadAlerts, 1400);
setInterval(loadAlerts, 30000);        // WebSocket handles alert updates
setTimeout(loadNotifyWidget, 1500);
setInterval(loadNotifyWidget, 30000);
setTimeout(loadOnCallWidget, 1600);
setInterval(loadOnCallWidget, 60000);
setTimeout(loadAuditWidget, 1700);
setInterval(loadAuditWidget, 60000);

// Initialize Cost Intelligence, DB Watch, and Cardinality
setTimeout(loadCostIntel, 1800);
setInterval(loadCostIntel, 60000); // Refresh every minute
setTimeout(loadDBWatch, 1900);
setInterval(loadDBWatch, 5000); // Refresh every 5 seconds
setTimeout(loadCardinality, 2000);
setInterval(loadCardinality, 30000); // Refresh every 30 seconds
