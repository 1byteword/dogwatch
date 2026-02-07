(function () {
    const body = document.body;
    if (!body || !body.classList.contains('grunge-site')) {
        return;
    }

    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
    const saveData = Boolean(connection && connection.saveData);
    const lowPower = (
        (typeof navigator.deviceMemory === 'number' && navigator.deviceMemory <= 4) ||
        (typeof navigator.hardwareConcurrency === 'number' && navigator.hardwareConcurrency <= 4)
    );
    const allowMotion = !reduceMotion && !saveData;

    if (!allowMotion || lowPower) {
        body.classList.add('effects-lite');
    }

    const currentPath = (window.location.pathname.split('/').pop() || 'index.html').toLowerCase();
    document.querySelectorAll('.masthead nav a[href]').forEach(function (link) {
        const href = (link.getAttribute('href') || '').toLowerCase();
        if (!href || href.charAt(0) === '#') {
            return;
        }
        if (href === currentPath || (currentPath === '' && href === 'index.html')) {
            link.classList.add('auto-active');
            link.setAttribute('aria-current', 'page');
        }
    });

    if (allowMotion && 'IntersectionObserver' in window) {
        body.classList.add('motion-enabled');

        const revealTargets = document.querySelectorAll('main section, main article, footer.panel');
        const io = new IntersectionObserver(function (entries) {
            entries.forEach(function (entry) {
                if (entry.isIntersecting) {
                    entry.target.classList.add('is-visible');
                    io.unobserve(entry.target);
                }
            });
        }, {
            threshold: 0.12,
            rootMargin: '0px 0px -6% 0px'
        });

        revealTargets.forEach(function (node) {
            if (node.getBoundingClientRect().top < window.innerHeight * 0.9) {
                node.classList.add('is-visible');
                return;
            }
            io.observe(node);
        });
    } else {
        document.querySelectorAll('main section, main article, footer.panel').forEach(function (node) {
            node.classList.add('is-visible');
        });
    }

    if (allowMotion && !lowPower) {
        const root = document.documentElement;
        let rafId = 0;
        let nextX = 0.5;
        let nextY = 0.5;

        const applyPointerOffset = function () {
            rafId = 0;
            root.style.setProperty('--grunge-mx', nextX.toFixed(4));
            root.style.setProperty('--grunge-my', nextY.toFixed(4));
        };

        window.addEventListener('pointermove', function (event) {
            nextX = event.clientX / window.innerWidth;
            nextY = event.clientY / window.innerHeight;
            if (!rafId) {
                rafId = window.requestAnimationFrame(applyPointerOffset);
            }
        }, { passive: true });
    }
})();
