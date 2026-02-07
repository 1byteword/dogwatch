import { api } from "../../core/api";
import { NotifyChannel, NotifyLog } from "./types";

export async function loadNotifyChannels(): Promise<NotifyChannel[]> {
  const raw = await api.get<unknown>("/api/notify/channels");
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const ch = item as Record<string, unknown>;
    return {
      id: String(ch.id || `channel-${idx}`),
      name: String(ch.name || "channel"),
      type: String(ch.type || "webhook"),
      enabled: Boolean(ch.enabled),
      successRate: Number(ch.success_rate || 0),
      lastError: String(ch.last_error || ""),
      updatedAt: ch.updated_at ? String(ch.updated_at) : undefined
    };
  });
}

export async function loadNotifyHistory(channelId = ""): Promise<NotifyLog[]> {
  const q = channelId ? `?channel_id=${encodeURIComponent(channelId)}` : "";
  const raw = await api.get<unknown>(`/api/notify/history${q}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const log = item as Record<string, unknown>;
    const notification = (log.notification || {}) as Record<string, unknown>;
    return {
      id: String(log.id || `log-${idx}`),
      channelName: String(log.channel_name || ""),
      channelType: String(log.channel_type || ""),
      status: String(log.status || "unknown"),
      title: String(notification.title || "Notification"),
      sentAt: log.sent_at ? String(log.sent_at) : undefined,
      responseTimeMs: Number(log.response_time_ms || 0)
    };
  });
}
