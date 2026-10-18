ALTER TABLE ai_credit_policy_versions
    ADD COLUMN provider_account_ids jsonb NOT NULL
    DEFAULT '{"openai":"default","assemblyai":"default","azure":"default"}'::jsonb;
