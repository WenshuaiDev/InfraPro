// Dedicated durable test storage: the generic suite uses a small ephemeral tmpfs.
export const ingestionTestEnv = {
  INFRAPRO_TEST_DATABASE_URL: 'postgres://ingestion_test:ingestion-business-test-only@127.0.0.1:56533/ingestion_test',
  INFRAPRO_INGESTION_TEST_BUSINESS_CONTAINER: 'infrapro-ingestion-lab-business-1',
};
