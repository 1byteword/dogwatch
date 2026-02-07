export interface NotifyChannel {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  successRate: number;
  lastError: string;
  updatedAt?: string;
}

export interface NotifyLog {
  id: string;
  channelName: string;
  channelType: string;
  status: string;
  title: string;
  sentAt?: string;
  responseTimeMs: number;
}
