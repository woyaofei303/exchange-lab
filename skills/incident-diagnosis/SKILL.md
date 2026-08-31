---
name: incident-diagnosis
description: Diagnose exchange-lab incidents using logs, metrics, traces, dashboards, SLOs, event receipts, reconciliation, and Runbooks. Use for stale price, lag, liquidation, funding, ledger, or availability incidents.
---

# Incident Diagnosis

1. State the symptom, affected SLO, time window, and user impact without guessing a cause.
2. Follow correlation ID across HTTP, Outbox, NATS, worker receipt, database state, and audit facts.
3. Compare price freshness, consumer lag, risk snapshot version, settlement lag, and reconciliation status.
4. Write competing hypotheses and the observation that would disprove each.
5. Prefer read-only queries and bounded time ranges; redact secrets and personal data.
6. Follow the relevant Runbook for mitigation, recovery, and rollback approval.
7. Record the timeline, evidence links, decision owner, and actual recovery measurements.
8. Turn a confirmed failure into a regression test and alert review.

AI diagnoses only. It must not delete data, run migrations, replay financial events, or deploy without human approval.
