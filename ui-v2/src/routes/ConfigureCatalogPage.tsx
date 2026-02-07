import { For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import { createCatalogService } from "../core/actions";
import { loadCatalogServices, loadCatalogStats } from "../domains/catalog/service";
import { CatalogService } from "../domains/catalog/types";

function tierTone(tier: CatalogService["tier"]) {
  if (tier === "critical") return "error";
  if (tier === "high") return "warn";
  if (tier === "medium") return "ok";
  return "neutral";
}

function healthTone(health: CatalogService["health"]) {
  if (health === "unhealthy") return "error";
  if (health === "degraded") return "warn";
  if (health === "healthy") return "ok";
  return "neutral";
}

export function ConfigureCatalogPage() {
  const [tier, setTier] = createSignal("");
  const [health, setHealth] = createSignal("");
  const [query, setQuery] = createSignal("");
  const [selectedId, setSelectedId] = createSignal("");
  const [showCreateService, setShowCreateService] = createSignal(false);
  const [showImport, setShowImport] = createSignal(false);
  const [serviceName, setServiceName] = createSignal("");
  const [serviceTeam, setServiceTeam] = createSignal("");
  const [serviceTier, setServiceTier] = createSignal<"critical" | "high" | "medium" | "low">("medium");
  const [serviceDescription, setServiceDescription] = createSignal("");
  const [importPayload, setImportPayload] = createSignal("");
  const [notice, setNotice] = createSignal("");
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal(Date.now());
  const now = createNowTicker(15000);

  const filters = createMemo(() => ({
    tier: tier(),
    health: health()
  }));

  const [services, { refetch }] = createResource(filters, loadCatalogServices);
  const [stats, { refetch: refetchStats }] = createResource(loadCatalogStats);
  const hasError = createMemo(() => Boolean(services.error || stats.error));

  createEffect(() => {
    const rows = services() || [];
    if (!rows.length) {
      setSelectedId("");
      return;
    }

    if (!selectedId() || !rows.some((svc) => svc.id === selectedId())) {
      setSelectedId(rows[0].id);
    }
  });

  createEffect(() => {
    if (services()) setLastUpdatedAt(Date.now());
  });

  useAutoRefresh(() => {
    refetch();
    refetchStats();
  }, 30000);

  const filtered = createMemo(() => {
    const q = query().trim().toLowerCase();
    if (!q) return services() || [];
    return (services() || []).filter((svc) => {
      const haystack = `${svc.displayName} ${svc.name} ${svc.teamName} ${svc.description}`.toLowerCase();
      return haystack.includes(q);
    });
  });

  const selected = createMemo(() => filtered().find((svc) => svc.id === selectedId()) ?? null);

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  function refreshAll() {
    refetch();
    refetchStats();
    setLastUpdatedAt(Date.now());
  }

  function openCreateService() {
    setServiceName("");
    setServiceTeam("");
    setServiceTier("medium");
    setServiceDescription("");
    setShowCreateService(true);
  }

  async function submitCreateService() {
    if (!serviceName().trim()) {
      setNotice("Service name is required.");
      return;
    }
    try {
      await createCatalogService({
        name: serviceName().trim(),
        displayName: serviceName().trim(),
        teamName: serviceTeam().trim(),
        tier: serviceTier(),
        description: serviceDescription().trim()
      });
      setShowCreateService(false);
      setNotice("Service created.");
      refreshAll();
    } catch {
      setNotice("Failed to create service.");
    }
  }

  async function submitImport() {
    let parsed: Array<Record<string, unknown>> = [];
    try {
      const raw = JSON.parse(importPayload()) as unknown;
      if (!Array.isArray(raw)) throw new Error("Import payload must be an array.");
      parsed = raw as Array<Record<string, unknown>>;
    } catch {
      setNotice("Import payload must be valid JSON array.");
      return;
    }

    if (parsed.length === 0) {
      setNotice("Nothing to import.");
      return;
    }

    let success = 0;
    for (const row of parsed) {
      const name = String(row.name || "").trim();
      if (!name) continue;
      try {
        await createCatalogService({
          name,
          displayName: String(row.displayName || row.display_name || name),
          description: String(row.description || ""),
          tier: (String(row.tier || "medium") as "critical" | "high" | "medium" | "low"),
          teamName: String(row.teamName || row.team_name || ""),
          ownerEmail: String(row.ownerEmail || row.owner_email || ""),
          repoUrl: String(row.repoUrl || row.repo_url || ""),
          docsUrl: String(row.docsUrl || row.docs_url || ""),
          runbookUrl: String(row.runbookUrl || row.runbook_url || "")
        });
        success += 1;
      } catch {
        // Continue best-effort import for remaining services.
      }
    }

    setShowImport(false);
    setImportPayload("");
    setNotice(`Imported ${success} service${success === 1 ? "" : "s"}.`);
    refreshAll();
  }

  return (
    <div class="page-grid">
      <Panel
        title="Service Catalog"
        actions={
          <>
            <Button onClick={() => setShowImport(true)}>Import</Button>
            <Button variant="primary" onClick={openCreateService}>Create Service</Button>
            <Button onClick={refreshAll}>Refresh</Button>
            <Badge tone="neutral">updated {updatedAgo()}</Badge>
          </>
        }
      >
        <Show when={hasError()}>
          <div class="inline-notice">Catalog API is unavailable right now.</div>
        </Show>
        <div class="catalog-stats-grid">
          <div class="catalog-stat-card">
            <label>Total</label>
            <strong>{stats()?.total || 0}</strong>
          </div>
          <div class="catalog-stat-card">
            <label>Critical</label>
            <strong>{stats()?.critical || 0}</strong>
          </div>
          <div class="catalog-stat-card">
            <label>Unhealthy</label>
            <strong>{stats()?.unhealthy || 0}</strong>
          </div>
          <div class="catalog-stat-card">
            <label>Healthy</label>
            <strong>{stats()?.healthy || 0}</strong>
          </div>
        </div>

        <div class="catalog-toolbar">
          <Input placeholder="Filter services..." value={query()} onInput={(e) => setQuery(e.currentTarget.value)} />
          <select class="input catalog-select" value={tier()} onChange={(e) => setTier(e.currentTarget.value)}>
            <option value="">all tiers</option>
            <option value="critical">critical</option>
            <option value="high">high</option>
            <option value="medium">medium</option>
            <option value="low">low</option>
          </select>
          <select class="input catalog-select" value={health()} onChange={(e) => setHealth(e.currentTarget.value)}>
            <option value="">all health</option>
            <option value="healthy">healthy</option>
            <option value="degraded">degraded</option>
            <option value="unhealthy">unhealthy</option>
          </select>
        </div>

        <Show when={!services.loading} fallback={<div class="paragraph">Loading services…</div>}>
          <div class="alert-list catalog-list" role="listbox" aria-label="Catalog services">
            <For each={filtered()}>
              {(svc) => (
                <button
                  class={`alert-row${selectedId() === svc.id ? " is-selected" : ""}`}
                  onClick={() => setSelectedId(svc.id)}
                >
                  <div class="alert-row-main">
                    <strong>{svc.displayName}</strong>
                    <span>{svc.teamName}</span>
                  </div>
                  <div class="alert-row-meta">
                    <Badge tone={tierTone(svc.tier)}>{svc.tier}</Badge>
                    <Badge tone={healthTone(svc.health)}>{svc.health}</Badge>
                    <span>{svc.incidentCount30d} incidents / 30d</span>
                  </div>
                </button>
              )}
            </For>
          </div>
        </Show>
        <Show when={notice()}>
          <div class="inline-notice">{notice()}</div>
        </Show>
      </Panel>

      <Panel
        title="Service Context"
        actions={<Badge tone={healthTone(selected()?.health || "unknown")}>{selected()?.health || "unknown"}</Badge>}
      >
        <Show when={selected()} fallback={<div class="paragraph">Select a service to view context.</div>}>
          <div class="detail-stack">
            <h3>{selected()!.displayName}</h3>
            <p class="paragraph">{selected()!.description || "No description set."}</p>

            <div class="kv-grid">
              <div>
                <label>Tier</label>
                <div>{selected()!.tier}</div>
              </div>
              <div>
                <label>Lifecycle</label>
                <div>{selected()!.lifecycle}</div>
              </div>
              <div>
                <label>Team</label>
                <div>{selected()!.teamName}</div>
              </div>
              <div>
                <label>Owner</label>
                <div>{selected()!.ownerEmail || "n/a"}</div>
              </div>
              <div>
                <label>Uptime 30d</label>
                <div>{selected()!.uptimePercent30d.toFixed(2)}%</div>
              </div>
              <div>
                <label>Avg Response</label>
                <div>{Math.round(selected()!.avgResponseTimeMs)}ms</div>
              </div>
            </div>

            <div class="row">
              <Show when={selected()!.repoUrl}>
                <a class="catalog-link" href={selected()!.repoUrl} target="_blank" rel="noreferrer">Repo</a>
              </Show>
              <Show when={selected()!.docsUrl}>
                <a class="catalog-link" href={selected()!.docsUrl} target="_blank" rel="noreferrer">Docs</a>
              </Show>
              <Show when={selected()!.runbookUrl}>
                <a class="catalog-link" href={selected()!.runbookUrl} target="_blank" rel="noreferrer">Runbook</a>
              </Show>
              <Show when={selected()!.slackChannel}>
                <Badge tone="neutral">{selected()!.slackChannel}</Badge>
              </Show>
            </div>
          </div>
        </Show>
      </Panel>

      <Show when={showCreateService()}>
        <div class="modal-overlay" onClick={() => setShowCreateService(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Create Service</h3>
            <div class="detail-stack">
              <Input value={serviceName()} onInput={(e) => setServiceName(e.currentTarget.value)} placeholder="service-name" />
              <Input value={serviceTeam()} onInput={(e) => setServiceTeam(e.currentTarget.value)} placeholder="Team" />
              <select
                class="input"
                value={serviceTier()}
                onChange={(e) => setServiceTier(e.currentTarget.value as "critical" | "high" | "medium" | "low")}
              >
                <option value="critical">critical</option>
                <option value="high">high</option>
                <option value="medium">medium</option>
                <option value="low">low</option>
              </select>
              <textarea
                class="input modal-textarea"
                value={serviceDescription()}
                onInput={(e) => setServiceDescription(e.currentTarget.value)}
                placeholder="Description"
              />
              <div class="row">
                <Button onClick={() => setShowCreateService(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitCreateService}>Create</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>

      <Show when={showImport()}>
        <div class="modal-overlay" onClick={() => setShowImport(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Import Services</h3>
            <div class="detail-stack">
              <p class="paragraph">
                Paste JSON array with fields like <span class="mono">name</span>, <span class="mono">tier</span>, <span class="mono">teamName</span>.
              </p>
              <textarea
                class="input modal-textarea"
                value={importPayload()}
                onInput={(e) => setImportPayload(e.currentTarget.value)}
                placeholder='[{"name":"checkout","tier":"critical","teamName":"payments"}]'
              />
              <div class="row">
                <Button onClick={() => setShowImport(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitImport}>Import</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </div>
  );
}
