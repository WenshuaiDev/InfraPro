-- Run after migration as DBA. The application migration grants safe table/view
-- privileges to this NOLOGIN role; provision a separate login and GRANT this
-- role to it using the installation's secret manager. Do not reuse the API user.
GRANT USAGE ON SCHEMA public TO infrapro_task_index;
-- No source-state UPDATE, source reports, notifications or account privileges.
