import { IncidentItem } from "./types";

export const mockIncidents: IncidentItem[] = [
  {
    id: "inc-1442",
    title: "Checkout degraded in us-east-1",
    severity: "critical",
    status: "triggered",
    service: "checkout-api",
    commander: "A. Rivera",
    responders: ["A. Rivera", "M. Chen", "S. Patel"],
    startedAt: "18m ago",
    startedAtRaw: new Date(Date.now() - 18 * 60 * 1000).toISOString(),
    timeline: [
      { id: "t1", time: "18m", kind: "alert", summary: "Latency p95 breach fired" },
      { id: "t2", time: "16m", kind: "deploy", summary: "Deploy 2026.02.07-rc4 observed" },
      { id: "t3", time: "11m", kind: "note", summary: "DB connections saturating in one shard" },
      { id: "t4", time: "6m", kind: "status", summary: "Mitigating via traffic shift 20%" }
    ]
  },
  {
    id: "inc-1439",
    title: "Order worker backlog elevated",
    severity: "high",
    status: "acknowledged",
    service: "order-worker",
    commander: "J. Owens",
    responders: ["J. Owens", "K. Singh"],
    startedAt: "42m ago",
    startedAtRaw: new Date(Date.now() - 42 * 60 * 1000).toISOString(),
    timeline: [
      { id: "t5", time: "42m", kind: "alert", summary: "Consumer lag alert triggered" },
      { id: "t6", time: "31m", kind: "note", summary: "Node pressure on two workers" },
      { id: "t7", time: "12m", kind: "status", summary: "Autoscaling floor increased" }
    ]
  }
];
