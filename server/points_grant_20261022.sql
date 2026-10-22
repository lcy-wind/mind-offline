-- Explicit one-off operation, never run automatically on application startup.
BEGIN;
WITH campaign AS (
 INSERT INTO schema_migrations(version) VALUES('20261022_registered_accounts_1000_v1')
 ON CONFLICT DO NOTHING RETURNING version
), grants AS (
 INSERT INTO point_grants(campaign,account_id,amount)
 SELECT c.version,a.id,1000 FROM campaign c CROSS JOIN accounts a
 ON CONFLICT DO NOTHING RETURNING account_id,amount
), credited AS (
 UPDATE guests g SET balance=g.balance+grants.amount FROM grants WHERE g.id=grants.account_id
 RETURNING grants.amount
)
SELECT count(*) AS credited_accounts,COALESCE(sum(amount),0) AS credited_points FROM credited;
COMMIT;
