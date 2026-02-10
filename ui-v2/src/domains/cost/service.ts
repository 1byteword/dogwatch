import { api } from "../../core/api";
import { CardinalityHotspot, CostEstimate, CostQuickWin, CostRecommendation } from "./types";

export async function loadCostEstimate(): Promise<CostEstimate | null> {
  try {
    const raw = await api.get<Partial<CostEstimate & {
      total_monthly?: number;
      datadog_equivalent?: number;
      savings_percent?: number;
    }>>("/api/cost/estimate");
    return {
      totalMonthly: Number(raw.totalMonthly ?? raw.total_monthly ?? 0),
      datadogEquivalent: Number(raw.datadogEquivalent ?? raw.datadog_equivalent ?? 0),
      savingsPercent: Number(raw.savingsPercent ?? raw.savings_percent ?? 0),
      breakdown: (raw.breakdown || []).map((item) => ({
        category: item.category || "other",
        amount: Number(item.amount || 0),
        unit: item.unit || "USD"
      }))
    };
  } catch {
    return null;
  }
}

export async function loadCardinalityHotspots(): Promise<CardinalityHotspot[]> {
  try {
    const raw = await api.get<Array<Partial<CardinalityHotspot & { growth_rate?: number }>>>("/api/cardinality/high");
    return (raw || []).map((row) => ({
      metric: row.metric || "unknown",
      series: Number(row.series || 0),
      labels: Number(row.labels || 0),
      growthRate: Number(row.growthRate ?? row.growth_rate ?? 0)
    }));
  } catch {
    return [];
  }
}

export async function loadCostRecommendations(): Promise<CostRecommendation[]> {
  try {
    const raw = await api.get<Array<Partial<CostRecommendation & { savings_estimate?: number }>>>("/api/cost/recommendations");
    return (raw || []).map((row) => ({
      id: row.id || "",
      title: row.title || "",
      description: row.description || "",
      impact: toImpact(row.impact),
      savingsEstimate: Number(row.savingsEstimate ?? row.savings_estimate ?? 0),
      effort: toEffort(row.effort)
    }));
  } catch {
    return [];
  }
}

export async function loadCostQuickWins(): Promise<CostQuickWin[]> {
  try {
    const raw = await api.get<Array<Partial<CostQuickWin & { monthly_savings?: number }>>>("/api/cost/quick-wins");
    return (raw || []).map((row, idx) => ({
      id: row.id || `qw-${idx}`,
      title: row.title || "",
      description: row.description || "",
      category: row.category || "general",
      monthlySavings: Number(row.monthlySavings ?? row.monthly_savings ?? 0),
      effort: toEffort(row.effort),
      impact: toImpact(row.impact)
    }));
  } catch {
    return [];
  }
}

function toImpact(s: string | undefined): CostRecommendation["impact"] {
  const v = (s || "").toLowerCase();
  if (v === "high" || v === "medium" || v === "low") return v;
  return "medium";
}

function toEffort(s: string | undefined): CostRecommendation["effort"] {
  const v = (s || "").toLowerCase();
  if (v === "trivial" || v === "easy" || v === "moderate") return v;
  return "easy";
}
