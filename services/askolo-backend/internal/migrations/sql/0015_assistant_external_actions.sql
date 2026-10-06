ALTER TABLE assistant_runs
  DROP CONSTRAINT assistant_runs_state_check,
  ADD CONSTRAINT assistant_runs_state_check
    CHECK (state IN (
      'planning',
      'needs_confirmation',
      'executing',
      'completed',
      'cancelled',
      'failed',
      'uncertain',
      'rejected'
    ));
