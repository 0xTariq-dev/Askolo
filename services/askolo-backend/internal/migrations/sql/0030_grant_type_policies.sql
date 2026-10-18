ALTER TABLE ai_credit_policy_versions
    ADD COLUMN grant_type_policies jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE ai_credit_policy_versions
SET grant_type_policies = jsonb_build_object(
    'signup', jsonb_build_object(
        'issuanceCapUsdMicros', signup_grant_usd_micros,
        'carryoverCapUsdMicros', GREATEST(signup_grant_usd_micros, rollover_cap_usd_micros),
        'expiryDays', rollover_expiry_days
    ),
    'monthly', jsonb_build_object(
        'issuanceCapUsdMicros', monthly_grant_usd_micros,
        'carryoverCapUsdMicros', GREATEST(monthly_grant_usd_micros, rollover_cap_usd_micros),
        'expiryDays', rollover_expiry_days
    ),
    'admin', jsonb_build_object(
        'issuanceCapUsdMicros', 9000000000000000,
        'carryoverCapUsdMicros', 9000000000000000,
        'expiryDays', rollover_expiry_days
    )
);

