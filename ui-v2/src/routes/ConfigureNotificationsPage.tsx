import { For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { createNotifyChannel, testNotifyChannel } from "../core/actions";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import { loadNotifyChannels, loadNotifyHistory } from "../domains/notify/service";

export function ConfigureNotificationsPage() {
  const [selectedId, setSelectedId] = createSignal("");
  const [showNewChannel, setShowNewChannel] = createSignal(false);
  const [channelName, setChannelName] = createSignal("");
  const [channelType, setChannelType] = createSignal<"webhook" | "slack">("webhook");
  const [channelTarget, setChannelTarget] = createSignal("");
  const [notice, setNotice] = createSignal("");
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal(Date.now());
  const now = createNowTicker(15000);

  const [channels, { refetch: refetchChannels }] = createResource(loadNotifyChannels);
  const selectedChannelId = createMemo(() => selectedId());
  const [history, { refetch: refetchHistory }] = createResource(selectedChannelId, loadNotifyHistory);
  const hasError = createMemo(() => Boolean(channels.error || history.error));

  createEffect(() => {
    const first = channels()?.[0]?.id;
    if (!selectedId() && first) setSelectedId(first);
  });

  useAutoRefresh(() => {
    refetchChannels();
    refetchHistory();
    setLastUpdatedAt(Date.now());
  }, 30000);

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  function refreshAll() {
    refetchChannels();
    refetchHistory();
    setLastUpdatedAt(Date.now());
  }

  async function submitNewChannel() {
    if (!channelName().trim() || !channelTarget().trim()) {
      setNotice("Channel name and target are required.");
      return;
    }
    try {
      const created = await createNotifyChannel({
        name: channelName().trim(),
        type: channelType(),
        target: channelTarget().trim()
      });
      setShowNewChannel(false);
      setNotice("Channel created.");
      if (created.id) setSelectedId(created.id);
      refreshAll();
    } catch {
      setNotice("Failed to create channel.");
    }
  }

  async function runTestChannel() {
    if (!selectedId()) {
      setNotice("Select a channel first.");
      return;
    }
    try {
      await testNotifyChannel(selectedId());
      setNotice("Test notification sent.");
      refetchHistory();
    } catch {
      setNotice("Failed to send test notification.");
    }
  }

  return (
    <div class="page-grid">
      <Panel
        title="Notification Channels"
        actions={
          <>
            <Button onClick={() => setShowNewChannel(true)}>New Channel</Button>
            <Button onClick={runTestChannel}>Test Channel</Button>
            <Button onClick={refreshAll}>Refresh</Button>
            <Badge tone="neutral">updated {updatedAgo()}</Badge>
          </>
        }
      >
        <Show when={hasError()}>
          <div class="inline-notice">Notifications backend is unavailable or not configured.</div>
        </Show>
        <Show when={!channels.loading} fallback={<div class="paragraph">Loading channels…</div>}>
          <div class="alert-list">
            <For each={channels() || []}>
              {(channel) => (
                <button
                  class={`alert-row${selectedId() === channel.id ? " is-selected" : ""}`}
                  onClick={() => setSelectedId(channel.id)}
                >
                  <div class="alert-row-main">
                    <strong>{channel.name}</strong>
                    <span>{channel.type}</span>
                  </div>
                  <div class="alert-row-meta">
                    <Badge tone={channel.enabled ? "ok" : "warn"}>{channel.enabled ? "enabled" : "disabled"}</Badge>
                    <span class="mono">{Math.round(channel.successRate)}% success</span>
                    <Show when={channel.lastError}><Badge tone="error">error</Badge></Show>
                  </div>
                </button>
              )}
            </For>
          </div>
        </Show>
      </Panel>

      <Panel title="Delivery History" actions={<Badge tone="neutral">{history()?.length || 0} events</Badge>}>
        <Show when={!history.loading} fallback={<div class="paragraph">Loading notifications…</div>}>
          <div class="dense-table">
            <div class="dense-head notif-grid"><span>Time</span><span>Channel</span><span>Status</span><span>Title</span><span>RT</span></div>
            <div class="dense-body short-body">
              <For each={history() || []}>
                {(log) => (
                  <div class="dense-row notif-grid">
                    <span class="mono">{log.sentAt ? new Date(log.sentAt).toLocaleTimeString() : "-"}</span>
                    <span>{log.channelName || log.channelType}</span>
                    <span><Badge tone={log.status === "failed" ? "error" : "ok"}>{log.status}</Badge></span>
                    <span class="truncate-text">{log.title}</span>
                    <span class="mono">{log.responseTimeMs}ms</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Panel>
      <Show when={notice()}>
        <div class="inline-notice">{notice()}</div>
      </Show>

      <Show when={showNewChannel()}>
        <div class="modal-overlay" onClick={() => setShowNewChannel(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>New Notification Channel</h3>
            <div class="detail-stack">
              <Input value={channelName()} onInput={(e) => setChannelName(e.currentTarget.value)} placeholder="Channel name" />
              <select
                class="input"
                value={channelType()}
                onChange={(e) => setChannelType(e.currentTarget.value as "webhook" | "slack")}
              >
                <option value="webhook">webhook</option>
                <option value="slack">slack</option>
              </select>
              <Input
                value={channelTarget()}
                onInput={(e) => setChannelTarget(e.currentTarget.value)}
                placeholder="https://..."
              />
              <div class="row">
                <Button onClick={() => setShowNewChannel(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitNewChannel}>Create</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </div>
  );
}

export default ConfigureNotificationsPage;
