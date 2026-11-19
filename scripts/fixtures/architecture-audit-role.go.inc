// The original resource tests create a new schema from Goose for every case.
// Complete its deployment before the unchanged restricted worker reads facts.
func installPublishedAuditSchemaRoles(t *testing.T, f resourceFixture, plan string) {
	t.Helper()
	body, err := os.ReadFile(plan)
	must(t, err)
	var inputs []struct{ Name, Path, SHA256 string }
	must(t, json.Unmarshal(body, &inputs))
	connection, err := f.env.Pool.Acquire(ctx)
	must(t, err)
	defer connection.Release()
	var database, schema string
	must(t, connection.QueryRow(ctx, "SELECT current_database(),current_schema()").Scan(&database, &schema))
	if !strings.HasSuffix(database, "_test") || !strings.HasPrefix(schema, "identity_test_") {
		t.Fatalf("unexpected audit fixture database or schema: %s/%s", database, schema)
	}
	for _, input := range inputs {
		bytes, err := os.ReadFile(input.Path)
		must(t, err)
		if fmt.Sprintf("%x", sha256.Sum256(bytes)) != input.SHA256 {
			t.Fatalf("published audit grant checksum changed: %s", input.Name)
		}
		_, err = connection.Exec(ctx, string(bytes), pgx.QueryExecModeSimpleProtocol)
		must(t, err)
		t.Logf("audit source fixture installed %s %s", input.Name, input.SHA256)
	}
}

func verifyPublishedAuditIdentity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var session, role, schema string
	var metadata, secrets, writes bool
	err := pool.QueryRow(ctx, `SELECT session_user,current_user,current_schema(),
		has_column_privilege(current_user,'engineering_resource_records','data','SELECT'),
		has_column_privilege(current_user,'account_passwords','password_hash','SELECT'),
		has_column_privilege(current_user,'accounts','status','UPDATE')`).Scan(&session,&role,&schema,&metadata,&secrets,&writes)
	must(t, err)
	if role != "infrapro_audit_worker" || !strings.HasPrefix(schema, "identity_test_") || !metadata || secrets || writes {
		t.Fatalf("unexpected restricted audit identity or privileges %s/%s/%s %t %t %t",session,role,schema,metadata,secrets,writes)
	}
	t.Logf("audit source fixture session_user=%s current_user=%s metadata_read=%t credential_read=%t account_write=%t",session,role,metadata,secrets,writes)
}
