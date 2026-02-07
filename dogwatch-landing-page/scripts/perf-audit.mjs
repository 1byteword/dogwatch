#!/usr/bin/env node

import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const HOST = "127.0.0.1";
const SITE_PORT = 4173;
const CDP_PORT = 9222;
const CHROME_BIN = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const PAGES = ["index.html", "features.html", "docs.html", "downloads.html", "pricing.html"];
const MIME_TYPES = {
    ".html": "text/html; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".js": "application/javascript; charset=utf-8",
    ".png": "image/png",
    ".webp": "image/webp",
    ".svg": "image/svg+xml",
    ".json": "application/json; charset=utf-8",
    ".woff2": "font/woff2"
};

function sleep(ms) {
    return new Promise((resolvePromise) => setTimeout(resolvePromise, ms));
}

async function waitForHttp(url, timeoutMs = 12000) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
        try {
            const res = await fetch(url);
            if (res.ok) {
                return;
            }
        } catch {
            // Continue polling.
        }
        await sleep(120);
    }
    throw new Error(`Timed out waiting for ${url}`);
}

class CDPClient {
    constructor(wsUrl) {
        this.wsUrl = wsUrl;
        this.ws = null;
        this.id = 0;
        this.pending = new Map();
        this.listeners = new Map();
    }

    async connect() {
        await new Promise((resolvePromise, rejectPromise) => {
            const ws = new WebSocket(this.wsUrl);
            this.ws = ws;

            ws.onopen = () => resolvePromise();
            ws.onerror = (event) => rejectPromise(new Error(`WebSocket error: ${String(event.message || "unknown")}`));
            ws.onmessage = (event) => {
                const msg = JSON.parse(event.data);
                if (msg.id) {
                    const pending = this.pending.get(msg.id);
                    if (pending) {
                        this.pending.delete(msg.id);
                        if (msg.error) {
                            pending.reject(new Error(msg.error.message || "CDP error"));
                        } else {
                            pending.resolve(msg.result);
                        }
                    }
                    return;
                }
                if (msg.method) {
                    const handlers = this.listeners.get(msg.method);
                    if (handlers) {
                        handlers.forEach((handler) => handler(msg.params || {}));
                    }
                }
            };
            ws.onclose = () => {
                this.pending.forEach((pending) => pending.reject(new Error("CDP socket closed")));
                this.pending.clear();
            };
        });
    }

    send(method, params = {}) {
        const id = ++this.id;
        const payload = JSON.stringify({ id, method, params });
        return new Promise((resolvePromise, rejectPromise) => {
            this.pending.set(id, { resolve: resolvePromise, reject: rejectPromise });
            this.ws.send(payload);
        });
    }

    on(method, handler) {
        if (!this.listeners.has(method)) {
            this.listeners.set(method, []);
        }
        this.listeners.get(method).push(handler);
    }

    waitFor(method, timeoutMs = 15000) {
        return new Promise((resolvePromise, rejectPromise) => {
            const timer = setTimeout(() => {
                rejectPromise(new Error(`Timed out waiting for ${method}`));
            }, timeoutMs);

            const handler = (params) => {
                clearTimeout(timer);
                const handlers = this.listeners.get(method) || [];
                this.listeners.set(
                    method,
                    handlers.filter((h) => h !== handler)
                );
                resolvePromise(params);
            };

            this.on(method, handler);
        });
    }

    close() {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.close();
        }
    }
}

function toMetricMap(metrics) {
    return Object.fromEntries(metrics.map((item) => [item.name, item.value]));
}

async function createTarget(url) {
    const endpoint = `http://${HOST}:${CDP_PORT}/json/new?${encodeURIComponent(url)}`;
    const response = await fetch(endpoint, { method: "PUT" });
    if (!response.ok) {
        throw new Error(`Failed to create target: ${response.status} ${response.statusText}`);
    }
    return response.json();
}

async function closeTarget(targetId) {
    await fetch(`http://${HOST}:${CDP_PORT}/json/close/${targetId}`);
}

async function auditPage(page) {
    const url = `http://${HOST}:${SITE_PORT}/${page}`;
    const target = await createTarget(url);
    const client = new CDPClient(target.webSocketDebuggerUrl);
    await client.connect();

    try {
        await client.send("Page.enable");
        await client.send("Runtime.enable");
        await client.send("Performance.enable");
        await client.send("Page.addScriptToEvaluateOnNewDocument", {
            source: `
                (() => {
                    window.__dwPerf = { lcp: 0, cls: 0 };
                    try {
                        new PerformanceObserver((list) => {
                            const entries = list.getEntries();
                            const last = entries[entries.length - 1];
                            if (last) {
                                window.__dwPerf.lcp = last.renderTime || last.loadTime || last.startTime || 0;
                            }
                        }).observe({ type: "largest-contentful-paint", buffered: true });
                    } catch {}
                    try {
                        new PerformanceObserver((list) => {
                            for (const entry of list.getEntries()) {
                                if (!entry.hadRecentInput) {
                                    window.__dwPerf.cls += entry.value;
                                }
                            }
                        }).observe({ type: "layout-shift", buffered: true });
                    } catch {}
                })();
            `
        });

        await client.send("Page.navigate", { url });
        await client.waitFor("Page.loadEventFired", 18000);
        await sleep(1700);

        const runtimeResult = await client.send("Runtime.evaluate", {
            expression: `
                (() => {
                    const nav = performance.getEntriesByType("navigation")[0];
                    const paints = performance.getEntriesByType("paint");
                    const paintByName = Object.fromEntries(paints.map((p) => [p.name, p.startTime]));
                    const resources = performance.getEntriesByType("resource");
                    const localResources = resources.filter((r) => r.name.startsWith(location.origin));
                    const localTransfer = localResources.reduce((sum, r) => sum + (r.transferSize || 0), 0);
                    const totalTransfer = resources.reduce((sum, r) => sum + (r.transferSize || 0), nav ? (nav.transferSize || 0) : 0);
                    return {
                        fcp: paintByName["first-contentful-paint"] || 0,
                        fp: paintByName["first-paint"] || 0,
                        lcp: window.__dwPerf ? window.__dwPerf.lcp : 0,
                        cls: window.__dwPerf ? window.__dwPerf.cls : 0,
                        resourceCount: resources.length,
                        localResourceCount: localResources.length,
                        nav: nav ? {
                            domContentLoaded: nav.domContentLoadedEventEnd,
                            load: nav.loadEventEnd,
                            transferSize: nav.transferSize,
                            encodedBodySize: nav.encodedBodySize
                        } : null,
                        localTransfer,
                        totalTransfer
                    };
                })();
            `,
            returnByValue: true
        });

        const perfMetrics = await client.send("Performance.getMetrics");
        const metricMap = toMetricMap(perfMetrics.metrics || []);

        return {
            page,
            fcp: runtimeResult.result.value.fcp,
            lcp: runtimeResult.result.value.lcp,
            cls: runtimeResult.result.value.cls,
            dcl: runtimeResult.result.value.nav ? runtimeResult.result.value.nav.domContentLoaded : 0,
            load: runtimeResult.result.value.nav ? runtimeResult.result.value.nav.load : 0,
            localTransferKb: (runtimeResult.result.value.localTransfer || 0) / 1024,
            totalTransferKb: (runtimeResult.result.value.totalTransfer || 0) / 1024,
            localResourceCount: runtimeResult.result.value.localResourceCount || 0,
            taskDurationMs: (metricMap.TaskDuration || 0) * 1000,
            scriptDurationMs: (metricMap.ScriptDuration || 0) * 1000,
            layoutDurationMs: (metricMap.LayoutDuration || 0) * 1000
        };
    } finally {
        client.close();
        await closeTarget(target.id);
    }
}

function fmt(ms) {
    return `${ms.toFixed(0)}ms`;
}

function fmtKb(kb) {
    return `${kb.toFixed(1)}KB`;
}

function getContentType(pathname) {
    const dot = pathname.lastIndexOf(".");
    if (dot === -1) {
        return "application/octet-stream";
    }
    const ext = pathname.slice(dot).toLowerCase();
    return MIME_TYPES[ext] || "application/octet-stream";
}

async function main() {
    const cwd = resolve(process.cwd());
    const chromeUserDir = mkdtempSync(join(tmpdir(), "dw-chrome-"));
    const server = createServer(async (req, res) => {
        try {
            const url = new URL(req.url || "/", `http://${HOST}:${SITE_PORT}`);
            let pathname = decodeURIComponent(url.pathname);
            if (pathname === "/") {
                pathname = "/index.html";
            }
            const diskPath = resolve(cwd, `.${pathname}`);
            if (!diskPath.startsWith(cwd)) {
                res.writeHead(403);
                res.end("Forbidden");
                return;
            }
            const data = await readFile(diskPath);
            res.writeHead(200, {
                "Content-Type": getContentType(diskPath),
                "Cache-Control": "no-store"
            });
            res.end(data);
        } catch {
            res.writeHead(404);
            res.end("Not Found");
        }
    });

    await new Promise((resolvePromise, rejectPromise) => {
        server.once("error", rejectPromise);
        server.listen(SITE_PORT, HOST, () => resolvePromise());
    });

    const chrome = spawn(
        CHROME_BIN,
        [
            "--headless=new",
            `--user-data-dir=${chromeUserDir}`,
            "--no-first-run",
            "--disable-extensions",
            "--disable-background-networking",
            "--disable-sync",
            "--metrics-recording-only",
            "--mute-audio",
            `--remote-debugging-port=${CDP_PORT}`,
            "about:blank"
        ],
        { stdio: "ignore" }
    );

    try {
        await waitForHttp(`http://${HOST}:${SITE_PORT}/index.html`, 10000);
        await waitForHttp(`http://${HOST}:${CDP_PORT}/json/version`, 12000);

        const results = [];
        for (const page of PAGES) {
            const row = await auditPage(page);
            results.push(row);
        }

        console.log("Page                FCP     LCP     CLS     DCL     Load    Local KB   Total KB  Local Rsrc  MainTask  Script  Layout");
        console.log("---------------------------------------------------------------------------------------------------------------");
        for (const row of results) {
            const name = row.page.padEnd(18, " ");
            console.log(
                `${name}${fmt(row.fcp).padEnd(8, " ")}${fmt(row.lcp).padEnd(8, " ")}${row.cls.toFixed(3).padEnd(8, " ")}` +
                `${fmt(row.dcl).padEnd(8, " ")}${fmt(row.load).padEnd(8, " ")}${fmtKb(row.localTransferKb).padEnd(11, " ")}` +
                `${fmtKb(row.totalTransferKb).padEnd(10, " ")}${String(row.localResourceCount).padEnd(12, " ")}` +
                `${fmt(row.taskDurationMs).padEnd(10, " ")}${fmt(row.scriptDurationMs).padEnd(8, " ")}${fmt(row.layoutDurationMs)}`
            );
        }
    } finally {
        await new Promise((resolvePromise) => {
            server.close(() => resolvePromise());
        });
        chrome.kill("SIGTERM");
        rmSync(chromeUserDir, { recursive: true, force: true });
    }
}

main().catch((error) => {
    console.error(error.message);
    process.exitCode = 1;
});
