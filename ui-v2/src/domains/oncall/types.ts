export interface OncallSchedule {
  id: string;
  name: string;
  description: string;
  timezone: string;
  teams: string[];
  layerCount: number;
  memberCount: number;
}

export interface OncallPolicy {
  id: string;
  name: string;
  description: string;
  ruleCount: number;
  repeatEnabled: boolean;
}

export interface OncallCurrent {
  userName: string;
  userId: string;
  startTime?: string;
  endTime?: string;
  isOverride: boolean;
}
