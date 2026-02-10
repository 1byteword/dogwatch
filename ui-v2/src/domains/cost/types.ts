export interface CostEstimate {
  totalMonthly: number;
  datadogEquivalent: number;
  savingsPercent: number;
  breakdown: CostBreakdownItem[];
}

export interface CostBreakdownItem {
  category: string;
  amount: number;
  unit: string;
}

export interface CardinalityHotspot {
  metric: string;
  series: number;
  labels: number;
  growthRate: number;
}

export interface CostRecommendation {
  id: string;
  title: string;
  description: string;
  impact: "high" | "medium" | "low";
  savingsEstimate: number;
  effort: "trivial" | "easy" | "moderate";
}

export interface CostQuickWin {
  id: string;
  title: string;
  description: string;
  category: string;
  monthlySavings: number;
  effort: "trivial" | "easy" | "moderate";
  impact: "high" | "medium" | "low";
}
