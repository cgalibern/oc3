-- 2026-11-02: a run cancelled, stopped or failed for good: it does not resume
-- nor go on past its step (POST /orchestration_runs/{run_id}/cancel).
ALTER TABLE orchestration_runs MODIFY status ENUM('running','succeeded','failed','stopped','cancelled') NOT NULL;
