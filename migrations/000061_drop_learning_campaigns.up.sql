-- ADR 0053: 000059 and 000060 copied historical campaign facts to the
-- book-anchored deck vocabulary state. The campaign tables are no longer
-- application state and can now be removed.
--
-- Take and verify a database backup before applying this destructive migration.
-- The down migration recreates empty tables and cannot restore deleted rows.
DROP TABLE learning_campaign_vocabulary;
DROP TABLE learning_campaigns;
