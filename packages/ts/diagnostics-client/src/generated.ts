export interface paths {
    "/api/diagnostics/v1/session/bootstrap": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__session_bootstrap"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/session/login": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["diagnostics_post__session_login"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/session": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__session"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/session/activity": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["diagnostics_post__session_activity"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/session/logout": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["diagnostics_post__session_logout"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/overview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__overview"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/nodes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__nodes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/nodes/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__nodes__id_"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/components": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__components"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/components/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__components__id_"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/replicas/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__replicas__id_"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/checks/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__checks__id_"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/transitions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__transitions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/metrics": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__metrics"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/diagnostics/v1/self": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["diagnostics_get__self"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/observer/v1/sessions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["observer_sessions"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/observer/v1/reports": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["observer_reports"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/healthz": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["healthz"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/readyz": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["readyz"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        Activity: {
            /** Format: date-time */
            idle_expires_at: string;
        };
        Auth: {
            revision: string;
            /** @constant */
            schema_version: 1;
            users: {
                enabled: boolean;
                id: string;
                password_hash: string;
                permissions: string[];
                username: string;
            }[];
        };
        Bindings: {
            allowed_target_hosts?: string[];
            assignment_revision: string;
            bindings: ({
                /** @constant */
                adapter: "clickhouse";
                address: string;
                credential_file?: string;
                database: string;
                id: string;
                tls?: boolean;
                tls_ca_file?: string;
                username: string;
            } | {
                /** @constant */
                adapter: "docker_container";
                expected_replica_id: string;
                id: string;
                selector_labels: {
                    [key: string]: string;
                };
                socket_path: string;
                stats_enabled?: boolean;
            } | {
                /** @constant */
                adapter: "host_resources";
                id: string;
                mount_paths?: {
                    id: string;
                    path: string;
                }[];
                network_interfaces?: {
                    id: string;
                    name: string;
                }[];
                scope: string;
            } | {
                /** @constant */
                adapter: "http_live";
                credential_file?: string;
                expected_json?: {
                    [key: string]: string | number | boolean | null;
                };
                expected_status: number;
                id: string;
                tls_ca_file?: string;
                url: string;
            } | {
                /** @constant */
                adapter: "http_ready";
                credential_file?: string;
                expected_json?: {
                    [key: string]: string | number | boolean | null;
                };
                expected_status: number;
                id: string;
                tls_ca_file?: string;
                url: string;
            } | {
                /** @constant */
                adapter: "jetstream_health";
                host: string;
                id: string;
                port: number;
            } | {
                /** @constant */
                adapter: "keeper_live";
                enabled?: boolean;
                host: string;
                id: string;
                port: number;
            } | {
                /** @constant */
                adapter: "native_process";
                allowed_root: string;
                expected_executable: string;
                expected_uid: number | null;
                id: string;
                pid_file?: string;
                pid_file_format?: string;
                report_file?: string;
            } | {
                /** @constant */
                adapter: "postgres";
                credential_file?: string;
                database: string;
                host: string;
                id: string;
                port: number;
                tls_ca_file?: string;
                tls_mode: string;
                username: string;
            } | {
                /** @constant */
                adapter: "proxy_listener";
                host?: string;
                id: string;
                port?: number;
            } | {
                /** @constant */
                adapter: "redis";
                address: string;
                credential_file?: string;
                database: number;
                id: string;
                tls?: boolean;
                tls_ca_file?: string;
                username?: string;
            } | {
                /** @constant */
                adapter: "s3";
                bucket: string;
                credential_file?: string;
                endpoint: string;
                id: string;
                region?: string;
                tls_ca_file?: string;
            } | {
                /** @constant */
                adapter: "self_report";
                allowed_root: string;
                dependency_key?: string;
                expected_executable: string;
                expected_replica_id: string;
                expected_uid: number | null;
                id: string;
                lane_id?: string;
                pid_namespace: string;
                report_file: string;
            } | {
                /** @constant */
                adapter: "systemd_unit";
                expected_uid?: number | null;
                id: string;
                unit_name: string;
            } | {
                /** @constant */
                adapter: "tcp";
                host: string;
                id: string;
                port: number;
                server_name?: string;
                tls?: boolean;
                tls_ca_file?: string;
            })[];
            instance_id: string;
            node_id: string;
            /** @constant */
            schema_version: 1;
        };
        Bootstrap: {
            csrf_token: string;
            initialized: boolean;
            login_allowed: boolean;
        };
        Check: {
            current_observation: {
                attempt_id: string | null;
                duration_ms: number | null;
                effective_age_ms: number | null;
                evidence: {
                    codes: string[];
                    exit_code?: number | null;
                    health?: string;
                    http_status?: number | null;
                    lanes?: {
                        active_since: string | null;
                        check_grace_ms: number;
                        deadline_at: string | null;
                        id: string;
                        last_attempt_at: string | null;
                        last_success_at: string | null;
                        next_attempt_at: string | null;
                        reason_code: string | null;
                        /** @enum {string} */
                        state: "idle" | "working" | "blocked";
                    }[];
                    oom_killed?: boolean | null;
                    restart_count?: number | null;
                    /** @enum {string} */
                    scope: "host" | "vm" | "container" | "none";
                    /** @enum {string} */
                    source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                    systemd_state?: string;
                    vantage_node_id: string;
                };
                facets: {
                    /** @enum {string} */
                    loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                    /** @enum {string} */
                    presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                    /** @enum {string} */
                    readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
                };
                /** @enum {string} */
                freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
                metrics: {
                    /** @enum {string} */
                    availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                    dimension_id?: string;
                    /** @enum {string} */
                    key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                    /** @enum {string} */
                    reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                    unit: string;
                    value: string | number | boolean | null;
                }[];
                observed_at: string | null;
                /** Format: uuid */
                observer_boot_id: string;
                /** @enum {string} */
                outcome: "pending" | "ok" | "failed" | "timeout" | "permission_denied" | "unsupported" | "not_configured" | "invalid_report";
                persistence: string;
                probe_id: string;
                process?: {
                    /** Format: uuid */
                    boot_id: string;
                    build_id?: string;
                    container_id?: string;
                    container_started_at?: string | null;
                    os_start_time_ms?: string;
                    pid?: number | null;
                    /** @enum {string} */
                    pid_namespace?: "host" | "container";
                    replica_id: string;
                    uid?: number | null;
                } | null;
                /** @enum {string} */
                reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                received_at: string | null;
                sample_age_ms: number | null;
            };
            last_success_at: string | null;
            probe_summary: {
                /** @enum {string} */
                adapter: "clickhouse" | "docker_container" | "host_resources" | "http_live" | "http_ready" | "jetstream_health" | "keeper_live" | "native_process" | "postgres" | "proxy_listener" | "redis" | "s3" | "self_report" | "systemd_unit" | "tcp";
                /** @enum {string} */
                facet: "presence" | "readiness" | "loop" | "resource";
                id: string;
                interval_ms: number;
                node_id: string;
                required: boolean;
                stale_after_ms: number;
                target_id: string;
                /** @enum {string} */
                target_kind: "node" | "replica" | "endpoint" | "dependency";
                timeout_ms: number;
            };
            reason: {
                code: string;
                hint: string;
                title: string;
            };
            related_ids: {
                [key: string]: string;
            };
        };
        ComponentDetail: {
            component: {
                coverage: {
                    attempted: number;
                    complete: boolean;
                    configured: number;
                    expected: number;
                    fresh: number;
                    missing_binding: number;
                    permission_denied: number;
                    probe_ids: {
                        [key: string]: string[];
                    };
                    /** @enum {string} */
                    state: "complete" | "partial" | "none";
                    unavailable: number;
                    unsupported: number;
                    vantage_mismatch: number;
                };
                disabled_replicas: number;
                display_name: string;
                /** @enum {string} */
                display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                expectation_reason?: string;
                expected_replicas: number;
                failed_replicas: number;
                id: string;
                /** @enum {string} */
                kind: "application" | "worker" | "database" | "cache" | "object_store" | "gateway" | "tool" | "diagnostics";
                min_ready_met: boolean;
                min_ready_replicas: number;
                observed_at: string | null;
                ready_replicas: number;
                reason_codes: string[];
                unknown_replicas: number;
            };
            coverage: {
                attempted: number;
                complete: boolean;
                configured: number;
                expected: number;
                fresh: number;
                missing_binding: number;
                permission_denied: number;
                probe_ids: {
                    [key: string]: string[];
                };
                /** @enum {string} */
                state: "complete" | "partial" | "none";
                unavailable: number;
                unsupported: number;
                vantage_mismatch: number;
            };
            dependency_summaries: {
                check: {
                    current_observation: {
                        attempt_id: string | null;
                        duration_ms: number | null;
                        effective_age_ms: number | null;
                        evidence: {
                            codes: string[];
                            exit_code?: number | null;
                            health?: string;
                            http_status?: number | null;
                            lanes?: {
                                active_since: string | null;
                                check_grace_ms: number;
                                deadline_at: string | null;
                                id: string;
                                last_attempt_at: string | null;
                                last_success_at: string | null;
                                next_attempt_at: string | null;
                                reason_code: string | null;
                                /** @enum {string} */
                                state: "idle" | "working" | "blocked";
                            }[];
                            oom_killed?: boolean | null;
                            restart_count?: number | null;
                            /** @enum {string} */
                            scope: "host" | "vm" | "container" | "none";
                            /** @enum {string} */
                            source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                            systemd_state?: string;
                            vantage_node_id: string;
                        };
                        facets: {
                            /** @enum {string} */
                            loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                            /** @enum {string} */
                            presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                            /** @enum {string} */
                            readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
                        };
                        /** @enum {string} */
                        freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
                        metrics: {
                            /** @enum {string} */
                            availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                            dimension_id?: string;
                            /** @enum {string} */
                            key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                            /** @enum {string} */
                            reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                            unit: string;
                            value: string | number | boolean | null;
                        }[];
                        observed_at: string | null;
                        /** Format: uuid */
                        observer_boot_id: string;
                        /** @enum {string} */
                        outcome: "pending" | "ok" | "failed" | "timeout" | "permission_denied" | "unsupported" | "not_configured" | "invalid_report";
                        persistence: string;
                        probe_id: string;
                        process?: {
                            /** Format: uuid */
                            boot_id: string;
                            build_id?: string;
                            container_id?: string;
                            container_started_at?: string | null;
                            os_start_time_ms?: string;
                            pid?: number | null;
                            /** @enum {string} */
                            pid_namespace?: "host" | "container";
                            replica_id: string;
                            uid?: number | null;
                        } | null;
                        /** @enum {string} */
                        reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                        received_at: string | null;
                        sample_age_ms: number | null;
                    };
                    last_success_at: string | null;
                    probe_summary: {
                        /** @enum {string} */
                        adapter: "clickhouse" | "docker_container" | "host_resources" | "http_live" | "http_ready" | "jetstream_health" | "keeper_live" | "native_process" | "postgres" | "proxy_listener" | "redis" | "s3" | "self_report" | "systemd_unit" | "tcp";
                        /** @enum {string} */
                        facet: "presence" | "readiness" | "loop" | "resource";
                        id: string;
                        interval_ms: number;
                        node_id: string;
                        required: boolean;
                        stale_after_ms: number;
                        target_id: string;
                        /** @enum {string} */
                        target_kind: "node" | "replica" | "endpoint" | "dependency";
                        timeout_ms: number;
                    };
                    reason: {
                        code: string;
                        hint: string;
                        title: string;
                    };
                    related_ids: {
                        [key: string]: string;
                    };
                };
                /** @enum {string} */
                evidence_mode: "self_report" | "shared_network" | "external_probe";
                from_replica_id: string;
                id: string;
                impact: string;
                probe_id: string;
                required: boolean;
                to_component_id: string;
            }[];
            replica_summaries: {
                coverage: {
                    attempted: number;
                    complete: boolean;
                    configured: number;
                    expected: number;
                    fresh: number;
                    missing_binding: number;
                    permission_denied: number;
                    probe_ids: {
                        [key: string]: string[];
                    };
                    /** @enum {string} */
                    state: "complete" | "partial" | "none";
                    unavailable: number;
                    unsupported: number;
                    vantage_mismatch: number;
                };
                /** @enum {string} */
                display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                /** @enum {string} */
                freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
                id: string;
                node_id: string;
                observed_at: string | null;
            }[];
        };
        ComponentView: {
            coverage: {
                attempted: number;
                complete: boolean;
                configured: number;
                expected: number;
                fresh: number;
                missing_binding: number;
                permission_denied: number;
                probe_ids: {
                    [key: string]: string[];
                };
                /** @enum {string} */
                state: "complete" | "partial" | "none";
                unavailable: number;
                unsupported: number;
                vantage_mismatch: number;
            };
            disabled_replicas: number;
            display_name: string;
            /** @enum {string} */
            display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
            /** @enum {string} */
            expectation: "continuous" | "on_demand" | "disabled" | "external";
            expectation_reason?: string;
            expected_replicas: number;
            failed_replicas: number;
            id: string;
            /** @enum {string} */
            kind: "application" | "worker" | "database" | "cache" | "object_store" | "gateway" | "tool" | "diagnostics";
            min_ready_met: boolean;
            min_ready_replicas: number;
            observed_at: string | null;
            ready_replicas: number;
            reason_codes: string[];
            unknown_replicas: number;
        };
        Coverage: {
            attempted: number;
            complete: boolean;
            configured: number;
            expected: number;
            fresh: number;
            missing_binding: number;
            permission_denied: number;
            probe_ids: {
                [key: string]: string[];
            };
            /** @enum {string} */
            state: "complete" | "partial" | "none";
            unavailable: number;
            unsupported: number;
            vantage_mismatch: number;
        };
        Credential: {
            access_key_id?: string;
            /** @enum {string} */
            kind: "password" | "bearer" | "s3";
            password?: string;
            /** @constant */
            schema_version: 1;
            secret_access_key?: string;
            session_token?: string;
            token?: string;
        };
        Keyring: {
            instance_id: string;
            keys: {
                assignment_revision: string;
                enabled: boolean;
                generation: string;
                key_id: string;
                node_id: string;
                secret_sha256: string;
            }[];
            revision: string;
            /** @constant */
            schema_version: 1;
        };
        LoginRequest: {
            password: string;
            username: string;
        };
        Manifest: {
            components: {
                display_name: string;
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                expectation_reason?: string;
                id: string;
                /** @enum {string} */
                kind: "application" | "worker" | "database" | "cache" | "object_store" | "gateway" | "tool" | "diagnostics";
                min_ready_replicas: number;
            }[];
            dependencies: {
                /** @enum {string} */
                evidence_mode: "self_report" | "shared_network" | "external_probe";
                from_replica_id: string;
                id: string;
                probe_id: string;
                required: boolean;
                to_component_id: string;
            }[];
            display_name: string;
            endpoints: {
                display_name: string;
                id: string;
                /** @enum {string} */
                purpose: "liveness" | "readiness" | "serving" | "storage" | "asset" | "control_plane";
                replica_id: string;
            }[];
            instance_id: string;
            nodes: {
                assignment_revision: string;
                display_name: string;
                enabled: boolean;
                id: string;
                /** @enum {string} */
                kind: "physical" | "vm" | "container" | "logical_test" | "external";
                parent_node_id?: string;
                /** @enum {string} */
                resource_scope: "host" | "vm" | "container" | "none";
            }[];
            probes: {
                /** @enum {string} */
                adapter: "clickhouse" | "docker_container" | "host_resources" | "http_live" | "http_ready" | "jetstream_health" | "keeper_live" | "native_process" | "postgres" | "proxy_listener" | "redis" | "s3" | "self_report" | "systemd_unit" | "tcp";
                binding_id: string;
                /** @enum {string} */
                facet: "presence" | "readiness" | "loop" | "resource";
                id: string;
                interval_ms: number;
                node_id: string;
                required: boolean;
                stale_after_ms: number;
                target_id: string;
                /** @enum {string} */
                target_kind: "node" | "replica" | "endpoint" | "dependency";
                timeout_ms: number;
            }[];
            replicas: {
                component_id: string;
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                id: string;
                node_id: string;
                required_probe_ids: string[];
            }[];
            revision: string;
            /** @constant */
            schema_version: 1;
        };
        Meta: {
            auth_revision?: string;
            diagnostics_state?: {
                [key: string]: string;
            };
            manifest_revision?: string;
            /** Format: uuid */
            request_id: string;
            /** Format: uuid */
            server_boot_id: string;
            /** Format: date-time */
            server_time: string;
            snapshot_id?: string;
        };
        Metric: {
            /** @enum {string} */
            availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
            dimension_id?: string;
            /** @enum {string} */
            key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
            /** @enum {string} */
            reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
            unit: string;
            value: string | number | boolean | null;
        };
        MetricSeries: {
            missing_intervals: string[];
            points: {
                /** Format: date-time */
                at: string;
                /** @enum {string} */
                availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                dimension_id?: string;
                /** @enum {string} */
                key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                node_id: string;
                /** @enum {string} */
                reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                /** @enum {string} */
                scope: "host" | "vm" | "container" | "none";
                /** @enum {string} */
                source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                target_id: string;
                /** @enum {string} */
                target_kind: "node" | "replica" | "endpoint" | "dependency";
                unit: string;
                value: string | number | boolean | null;
            }[];
            retention: {
                access_days: number;
                access_limit: number;
                actual_from: string | null;
                evicted_count: string;
                metric_hours: number;
                metric_limit: number;
                points_per_series: number;
                transition_hours: number;
                transition_limit: number;
            };
            /** @enum {string} */
            scope: "host" | "vm" | "container" | "none";
            /** @enum {string} */
            source: "os" | "systemd" | "docker" | "protocol" | "self_report";
            unit: string;
        };
        NodeDetail: {
            coverage: {
                attempted: number;
                complete: boolean;
                configured: number;
                expected: number;
                fresh: number;
                missing_binding: number;
                permission_denied: number;
                probe_ids: {
                    [key: string]: string[];
                };
                /** @enum {string} */
                state: "complete" | "partial" | "none";
                unavailable: number;
                unsupported: number;
                vantage_mismatch: number;
            };
            node: {
                assignment_revision: string;
                coverage: {
                    attempted: number;
                    complete: boolean;
                    configured: number;
                    expected: number;
                    fresh: number;
                    missing_binding: number;
                    permission_denied: number;
                    probe_ids: {
                        [key: string]: string[];
                    };
                    /** @enum {string} */
                    state: "complete" | "partial" | "none";
                    unavailable: number;
                    unsupported: number;
                    vantage_mismatch: number;
                };
                display_name: string;
                /** @enum {string} */
                display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
                enabled: boolean;
                id: string;
                /** @enum {string} */
                kind: "physical" | "vm" | "container" | "logical_test" | "external";
                metrics: {
                    /** Format: date-time */
                    at: string;
                    /** @enum {string} */
                    availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                    dimension_id?: string;
                    /** @enum {string} */
                    key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                    node_id: string;
                    /** @enum {string} */
                    reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                    /** @enum {string} */
                    scope: "host" | "vm" | "container" | "none";
                    /** @enum {string} */
                    source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                    target_id: string;
                    /** @enum {string} */
                    target_kind: "node" | "replica" | "endpoint" | "dependency";
                    unit: string;
                    value: string | number | boolean | null;
                }[];
                observed_at: string | null;
                observer: {
                    /** Format: uuid */
                    boot_id: string;
                    last_received_at: string | null;
                    online: boolean;
                    /** @enum {string} */
                    reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                };
                parent_node_id?: string;
                /** @enum {string} */
                resource_scope: "host" | "vm" | "container" | "none";
            };
            observer: {
                /** Format: uuid */
                boot_id: string;
                last_received_at: string | null;
                online: boolean;
                /** @enum {string} */
                reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
            };
            replica_counts: {
                [key: string]: number;
            };
            replica_summaries: {
                coverage: {
                    attempted: number;
                    complete: boolean;
                    configured: number;
                    expected: number;
                    fresh: number;
                    missing_binding: number;
                    permission_denied: number;
                    probe_ids: {
                        [key: string]: string[];
                    };
                    /** @enum {string} */
                    state: "complete" | "partial" | "none";
                    unavailable: number;
                    unsupported: number;
                    vantage_mismatch: number;
                };
                /** @enum {string} */
                display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                /** @enum {string} */
                freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
                id: string;
                node_id: string;
                observed_at: string | null;
            }[];
            resource_series_summary: {
                /** Format: date-time */
                at: string;
                /** @enum {string} */
                availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                dimension_id?: string;
                /** @enum {string} */
                key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                node_id: string;
                /** @enum {string} */
                reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                /** @enum {string} */
                scope: "host" | "vm" | "container" | "none";
                /** @enum {string} */
                source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                target_id: string;
                /** @enum {string} */
                target_kind: "node" | "replica" | "endpoint" | "dependency";
                unit: string;
                value: string | number | boolean | null;
            }[];
        };
        NodeView: {
            assignment_revision: string;
            coverage: {
                attempted: number;
                complete: boolean;
                configured: number;
                expected: number;
                fresh: number;
                missing_binding: number;
                permission_denied: number;
                probe_ids: {
                    [key: string]: string[];
                };
                /** @enum {string} */
                state: "complete" | "partial" | "none";
                unavailable: number;
                unsupported: number;
                vantage_mismatch: number;
            };
            display_name: string;
            /** @enum {string} */
            display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
            enabled: boolean;
            id: string;
            /** @enum {string} */
            kind: "physical" | "vm" | "container" | "logical_test" | "external";
            metrics: {
                /** Format: date-time */
                at: string;
                /** @enum {string} */
                availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                dimension_id?: string;
                /** @enum {string} */
                key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                node_id: string;
                /** @enum {string} */
                reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                /** @enum {string} */
                scope: "host" | "vm" | "container" | "none";
                /** @enum {string} */
                source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                target_id: string;
                /** @enum {string} */
                target_kind: "node" | "replica" | "endpoint" | "dependency";
                unit: string;
                value: string | number | boolean | null;
            }[];
            observed_at: string | null;
            observer: {
                /** Format: uuid */
                boot_id: string;
                last_received_at: string | null;
                online: boolean;
                /** @enum {string} */
                reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
            };
            parent_node_id?: string;
            /** @enum {string} */
            resource_scope: "host" | "vm" | "container" | "none";
        };
        Observation: {
            attempt_id: string | null;
            duration_ms: number | null;
            evidence: {
                codes: string[];
                exit_code?: number | null;
                health?: string;
                http_status?: number | null;
                lanes?: {
                    active_since: string | null;
                    check_grace_ms: number;
                    deadline_at: string | null;
                    id: string;
                    last_attempt_at: string | null;
                    last_success_at: string | null;
                    next_attempt_at: string | null;
                    reason_code: string | null;
                    /** @enum {string} */
                    state: "idle" | "working" | "blocked";
                }[];
                oom_killed?: boolean | null;
                restart_count?: number | null;
                /** @enum {string} */
                scope: "host" | "vm" | "container" | "none";
                /** @enum {string} */
                source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                systemd_state?: string;
                vantage_node_id: string;
            };
            facets: {
                /** @enum {string} */
                loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                /** @enum {string} */
                presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                /** @enum {string} */
                readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
            };
            metrics: {
                /** @enum {string} */
                availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                dimension_id?: string;
                /** @enum {string} */
                key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                /** @enum {string} */
                reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                unit: string;
                value: string | number | boolean | null;
            }[];
            observed_at: string | null;
            /** Format: uuid */
            observer_boot_id: string;
            /** @enum {string} */
            outcome: "pending" | "ok" | "failed" | "timeout" | "permission_denied" | "unsupported" | "not_configured" | "invalid_report";
            probe_id: string;
            process?: {
                /** Format: uuid */
                boot_id: string;
                build_id?: string;
                container_id?: string;
                container_started_at?: string | null;
                os_start_time_ms?: string;
                pid?: number | null;
                /** @enum {string} */
                pid_namespace?: "host" | "container";
                replica_id: string;
                uid?: number | null;
            } | null;
            /** @enum {string} */
            reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
            sample_age_ms: number | null;
        };
        ObserverAck: {
            accepted_sequence: string;
            config_revision: string;
            duplicate: boolean;
            /** Format: date-time */
            received_at: string;
            /** Format: uuid */
            server_boot_id: string;
        };
        ObserverConfig: {
            assignment_revision: string;
            bindings_file: string;
            credential_file: string;
            instance_id: string;
            manifest_file: string;
            node_id: string;
            /** @constant */
            schema_version: 1;
            server_url: string;
            state_dir: string;
            tls_ca_file?: string;
        };
        ObserverFrame: {
            assignment_revision: string;
            /** Format: uuid */
            boot_id: string;
            epoch: string;
            /** Format: date-time */
            generated_at: string;
            instance_id: string;
            manifest_revision: string;
            node_id: string;
            observations: {
                attempt_id: string | null;
                duration_ms: number | null;
                evidence: {
                    codes: string[];
                    exit_code?: number | null;
                    health?: string;
                    http_status?: number | null;
                    lanes?: {
                        active_since: string | null;
                        check_grace_ms: number;
                        deadline_at: string | null;
                        id: string;
                        last_attempt_at: string | null;
                        last_success_at: string | null;
                        next_attempt_at: string | null;
                        reason_code: string | null;
                        /** @enum {string} */
                        state: "idle" | "working" | "blocked";
                    }[];
                    oom_killed?: boolean | null;
                    restart_count?: number | null;
                    /** @enum {string} */
                    scope: "host" | "vm" | "container" | "none";
                    /** @enum {string} */
                    source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                    systemd_state?: string;
                    vantage_node_id: string;
                };
                facets: {
                    /** @enum {string} */
                    loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                    /** @enum {string} */
                    presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                    /** @enum {string} */
                    readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
                };
                metrics: {
                    /** @enum {string} */
                    availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                    dimension_id?: string;
                    /** @enum {string} */
                    key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                    /** @enum {string} */
                    reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                    unit: string;
                    value: string | number | boolean | null;
                }[];
                observed_at: string | null;
                /** Format: uuid */
                observer_boot_id: string;
                /** @enum {string} */
                outcome: "pending" | "ok" | "failed" | "timeout" | "permission_denied" | "unsupported" | "not_configured" | "invalid_report";
                probe_id: string;
                process?: {
                    /** Format: uuid */
                    boot_id: string;
                    build_id?: string;
                    container_id?: string;
                    container_started_at?: string | null;
                    os_start_time_ms?: string;
                    pid?: number | null;
                    /** @enum {string} */
                    pid_namespace?: "host" | "container";
                    replica_id: string;
                    uid?: number | null;
                } | null;
                /** @enum {string} */
                reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                sample_age_ms: number | null;
            }[];
            /** @constant */
            protocol_major: 1;
            sequence: string;
        };
        ObserverKey: {
            generation: string;
            instance_id: string;
            key_id: string;
            node_id: string;
            /** @constant */
            schema_version: 1;
            secret: string;
        };
        ObserverSession: {
            /** Format: date-time */
            expires_at: string;
            last_accepted_sequence: string;
            min_report_interval_ms: number;
            probe_ids: string[];
            /** Format: uuid */
            server_boot_id: string;
            token: string;
        };
        ObserverSessionRequest: {
            assignment_revision: string;
            /** Format: uuid */
            boot_id: string;
            epoch: string;
            instance_id: string;
            manifest_revision: string;
            node_id: string;
            /** @constant */
            protocol_major: 1;
        };
        Overview: {
            abnormal_components: {
                coverage: {
                    attempted: number;
                    complete: boolean;
                    configured: number;
                    expected: number;
                    fresh: number;
                    missing_binding: number;
                    permission_denied: number;
                    probe_ids: {
                        [key: string]: string[];
                    };
                    /** @enum {string} */
                    state: "complete" | "partial" | "none";
                    unavailable: number;
                    unsupported: number;
                    vantage_mismatch: number;
                };
                disabled_replicas: number;
                display_name: string;
                /** @enum {string} */
                display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                expectation_reason?: string;
                expected_replicas: number;
                failed_replicas: number;
                id: string;
                /** @enum {string} */
                kind: "application" | "worker" | "database" | "cache" | "object_store" | "gateway" | "tool" | "diagnostics";
                min_ready_met: boolean;
                min_ready_replicas: number;
                observed_at: string | null;
                ready_replicas: number;
                reason_codes: string[];
                unknown_replicas: number;
            }[];
            coverage: {
                attempted: number;
                complete: boolean;
                configured: number;
                expected: number;
                fresh: number;
                missing_binding: number;
                permission_denied: number;
                probe_ids: {
                    [key: string]: string[];
                };
                /** @enum {string} */
                state: "complete" | "partial" | "none";
                unavailable: number;
                unsupported: number;
                vantage_mismatch: number;
            };
            instance: {
                display_name: string;
                id: string;
            };
            min_refresh_interval_ms: number;
            observer_summary: {
                [key: string]: number;
            };
            recent_transitions: {
                /** Format: date-time */
                at: string;
                from_state: string;
                id: string;
                observation_ref: string | null;
                probe_id?: string;
                /** @enum {string} */
                reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                retired: boolean;
                target_id: string;
                /** @enum {string} */
                target_kind: "node" | "replica" | "endpoint" | "dependency";
                to_state: string;
            }[];
            resource_warnings: {
                dimension_id?: string;
                node_id: string;
                probe_id: string;
                /** @enum {string} */
                reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
            }[];
            summary: {
                [key: string]: number;
            };
        };
        ProcessReport: {
            dependencies: {
                /** Format: date-time */
                checked_at: string;
                id: string;
                reason_code: string | null;
                state: string;
            }[];
            /** Format: date-time */
            emitted_at: string;
            instance_id: string;
            lanes: {
                active_since: string | null;
                check_grace_ms: number;
                deadline_at: string | null;
                id: string;
                last_attempt_at: string | null;
                last_success_at: string | null;
                next_attempt_at: string | null;
                reason_code: string | null;
                /** @enum {string} */
                state: "idle" | "working" | "blocked";
            }[];
            metrics?: {
                dimension_id?: string;
                /** @enum {string} */
                key: "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "parser.duration_ms" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth";
                value: number;
            }[];
            node_id: string;
            /** @enum {string} */
            phase: "starting" | "ready" | "unconfigured" | "stopping" | "failed";
            process: {
                /** Format: uuid */
                boot_id: string;
                build_id: string;
                os_start_time_ms: string;
                pid: number;
                /** @enum {string} */
                pid_namespace: "host" | "container";
            };
            replica_id: string;
            report_seq: string;
            /** @constant */
            schema_version: 1;
            uptime_ms: string;
        };
        ReplicaView: {
            checks: {
                current_observation: {
                    attempt_id: string | null;
                    duration_ms: number | null;
                    effective_age_ms: number | null;
                    evidence: {
                        codes: string[];
                        exit_code?: number | null;
                        health?: string;
                        http_status?: number | null;
                        lanes?: {
                            active_since: string | null;
                            check_grace_ms: number;
                            deadline_at: string | null;
                            id: string;
                            last_attempt_at: string | null;
                            last_success_at: string | null;
                            next_attempt_at: string | null;
                            reason_code: string | null;
                            /** @enum {string} */
                            state: "idle" | "working" | "blocked";
                        }[];
                        oom_killed?: boolean | null;
                        restart_count?: number | null;
                        /** @enum {string} */
                        scope: "host" | "vm" | "container" | "none";
                        /** @enum {string} */
                        source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                        systemd_state?: string;
                        vantage_node_id: string;
                    };
                    facets: {
                        /** @enum {string} */
                        loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                        /** @enum {string} */
                        presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                        /** @enum {string} */
                        readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
                    };
                    /** @enum {string} */
                    freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
                    metrics: {
                        /** @enum {string} */
                        availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                        dimension_id?: string;
                        /** @enum {string} */
                        key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                        /** @enum {string} */
                        reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                        unit: string;
                        value: string | number | boolean | null;
                    }[];
                    observed_at: string | null;
                    /** Format: uuid */
                    observer_boot_id: string;
                    /** @enum {string} */
                    outcome: "pending" | "ok" | "failed" | "timeout" | "permission_denied" | "unsupported" | "not_configured" | "invalid_report";
                    persistence: string;
                    probe_id: string;
                    process?: {
                        /** Format: uuid */
                        boot_id: string;
                        build_id?: string;
                        container_id?: string;
                        container_started_at?: string | null;
                        os_start_time_ms?: string;
                        pid?: number | null;
                        /** @enum {string} */
                        pid_namespace?: "host" | "container";
                        replica_id: string;
                        uid?: number | null;
                    } | null;
                    /** @enum {string} */
                    reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                    received_at: string | null;
                    sample_age_ms: number | null;
                };
                last_success_at: string | null;
                probe_summary: {
                    /** @enum {string} */
                    adapter: "clickhouse" | "docker_container" | "host_resources" | "http_live" | "http_ready" | "jetstream_health" | "keeper_live" | "native_process" | "postgres" | "proxy_listener" | "redis" | "s3" | "self_report" | "systemd_unit" | "tcp";
                    /** @enum {string} */
                    facet: "presence" | "readiness" | "loop" | "resource";
                    id: string;
                    interval_ms: number;
                    node_id: string;
                    required: boolean;
                    stale_after_ms: number;
                    target_id: string;
                    /** @enum {string} */
                    target_kind: "node" | "replica" | "endpoint" | "dependency";
                    timeout_ms: number;
                };
                reason: {
                    code: string;
                    hint: string;
                    title: string;
                };
                related_ids: {
                    [key: string]: string;
                };
            }[];
            coverage: {
                attempted: number;
                complete: boolean;
                configured: number;
                expected: number;
                fresh: number;
                missing_binding: number;
                permission_denied: number;
                probe_ids: {
                    [key: string]: string[];
                };
                /** @enum {string} */
                state: "complete" | "partial" | "none";
                unavailable: number;
                unsupported: number;
                vantage_mismatch: number;
            };
            dependencies: {
                check: {
                    current_observation: {
                        attempt_id: string | null;
                        duration_ms: number | null;
                        effective_age_ms: number | null;
                        evidence: {
                            codes: string[];
                            exit_code?: number | null;
                            health?: string;
                            http_status?: number | null;
                            lanes?: {
                                active_since: string | null;
                                check_grace_ms: number;
                                deadline_at: string | null;
                                id: string;
                                last_attempt_at: string | null;
                                last_success_at: string | null;
                                next_attempt_at: string | null;
                                reason_code: string | null;
                                /** @enum {string} */
                                state: "idle" | "working" | "blocked";
                            }[];
                            oom_killed?: boolean | null;
                            restart_count?: number | null;
                            /** @enum {string} */
                            scope: "host" | "vm" | "container" | "none";
                            /** @enum {string} */
                            source: "os" | "systemd" | "docker" | "protocol" | "self_report";
                            systemd_state?: string;
                            vantage_node_id: string;
                        };
                        facets: {
                            /** @enum {string} */
                            loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                            /** @enum {string} */
                            presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                            /** @enum {string} */
                            readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
                        };
                        /** @enum {string} */
                        freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
                        metrics: {
                            /** @enum {string} */
                            availability: "available" | "unlimited" | "unsupported" | "permission_denied" | "not_configured" | "error";
                            dimension_id?: string;
                            /** @enum {string} */
                            key: "container.cpu_cores" | "container.cpu_limit_cores" | "container.memory_limit_bytes" | "container.memory_used_bytes" | "cpu.utilization_ratio" | "disk.available_bytes" | "disk.total_bytes" | "disk.used_ratio" | "dispatch.rejected_fences" | "event.consumer_position" | "event.delivery_lag_ms" | "event.outbox_oldest_ms" | "event.outbox_pending" | "event.pg_reconciliations" | "event.position_gaps" | "event.publish_acks" | "event.quarantined" | "event.redeliveries" | "memory.available_bytes" | "memory.total_bytes" | "memory.used_bytes" | "network.rx_bytes_per_second" | "network.tx_bytes_per_second" | "parser.duration_ms" | "process.cpu_cores" | "process.restart_count" | "process.rss_bytes" | "quota.ch_connections" | "quota.http_inflight" | "quota.parser_slots" | "quota.pg_connections" | "quota.s3_inflight" | "tenant.queue_depth" | "uptime_ms";
                            /** @enum {string} */
                            reason_code?: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                            unit: string;
                            value: string | number | boolean | null;
                        }[];
                        observed_at: string | null;
                        /** Format: uuid */
                        observer_boot_id: string;
                        /** @enum {string} */
                        outcome: "pending" | "ok" | "failed" | "timeout" | "permission_denied" | "unsupported" | "not_configured" | "invalid_report";
                        persistence: string;
                        probe_id: string;
                        process?: {
                            /** Format: uuid */
                            boot_id: string;
                            build_id?: string;
                            container_id?: string;
                            container_started_at?: string | null;
                            os_start_time_ms?: string;
                            pid?: number | null;
                            /** @enum {string} */
                            pid_namespace?: "host" | "container";
                            replica_id: string;
                            uid?: number | null;
                        } | null;
                        /** @enum {string} */
                        reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
                        received_at: string | null;
                        sample_age_ms: number | null;
                    };
                    last_success_at: string | null;
                    probe_summary: {
                        /** @enum {string} */
                        adapter: "clickhouse" | "docker_container" | "host_resources" | "http_live" | "http_ready" | "jetstream_health" | "keeper_live" | "native_process" | "postgres" | "proxy_listener" | "redis" | "s3" | "self_report" | "systemd_unit" | "tcp";
                        /** @enum {string} */
                        facet: "presence" | "readiness" | "loop" | "resource";
                        id: string;
                        interval_ms: number;
                        node_id: string;
                        required: boolean;
                        stale_after_ms: number;
                        target_id: string;
                        /** @enum {string} */
                        target_kind: "node" | "replica" | "endpoint" | "dependency";
                        timeout_ms: number;
                    };
                    reason: {
                        code: string;
                        hint: string;
                        title: string;
                    };
                    related_ids: {
                        [key: string]: string;
                    };
                };
                /** @enum {string} */
                evidence_mode: "self_report" | "shared_network" | "external_probe";
                from_replica_id: string;
                id: string;
                impact: string;
                probe_id: string;
                required: boolean;
                to_component_id: string;
            }[];
            /** @enum {string} */
            display_state: "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle";
            endpoints: {
                display_name: string;
                id: string;
                /** @enum {string} */
                purpose: "liveness" | "readiness" | "serving" | "storage" | "asset" | "control_plane";
                replica_id: string;
            }[];
            facets: {
                /** @enum {string} */
                loop: "idle" | "working" | "blocked" | "stalled" | "unknown" | "not_applicable";
                /** @enum {string} */
                presence: "running" | "stopped" | "missing" | "unknown" | "not_applicable";
                /** @enum {string} */
                readiness: "ready" | "not_ready" | "starting" | "unconfigured" | "unknown" | "not_applicable";
            };
            /** @enum {string} */
            freshness: "fresh" | "stale" | "never" | "historical" | "clock_uncertain";
            observed_at: string | null;
            process: {
                /** Format: uuid */
                boot_id: string;
                build_id?: string;
                container_id?: string;
                container_started_at?: string | null;
                os_start_time_ms?: string;
                pid?: number | null;
                /** @enum {string} */
                pid_namespace?: "host" | "container";
                replica_id: string;
                uid?: number | null;
            } | null;
            reason_codes: string[];
            replica: {
                component_id: string;
                /** @enum {string} */
                expectation: "continuous" | "on_demand" | "disabled" | "external";
                id: string;
                node_id: string;
                required_probe_ids: string[];
            };
        };
        Retention: {
            access_days: number;
            access_limit: number;
            actual_from: string | null;
            evicted_count: string;
            metric_hours: number;
            metric_limit: number;
            points_per_series: number;
            transition_hours: number;
            transition_limit: number;
        };
        ServerConfig: {
            allowed_hosts: string[];
            auth_file: string;
            development_origins?: string[];
            instance_id: string;
            limits: {
                concurrent_probes?: number;
                history_points?: number;
                max_nodes?: number;
                max_probes?: number;
                max_replicas?: number;
                probes_per_node?: number;
                store_max_bytes?: number;
            };
            listen: string;
            local_observer: {
                assignment_revision: string;
                bindings_file: string;
                node_id: string;
            } | null;
            manifest_file: string;
            node_keys_file: string;
            public_origin: string;
            resource_hints: {
                cpu_ratio: number;
                cpu_samples: number;
                disk_available_bytes: string;
                disk_available_ratio: number;
                enabled: boolean;
                memory_available_ratio: number;
            };
            /** @constant */
            schema_version: 1;
            state_dir: string;
            tls: {
                cert_file: string;
                key_file: string;
            } | null;
            trusted_proxy_cidrs: string[];
        };
        Session: {
            csrf_token: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            idle_expires_at: string;
            user: {
                id: string;
                permissions: string[];
                username: string;
            };
        };
        Transition: {
            /** Format: date-time */
            at: string;
            from_state: string;
            id: string;
            observation_ref: string | null;
            probe_id?: string;
            /** @enum {string} */
            reason_code: "adapter_blocked" | "adapter_error" | "adapter_unsupported" | "ambiguous_identity" | "assignment_mismatch" | "auth_configuration_unavailable" | "authentication_failed" | "authentication_required" | "broker_unavailable" | "check_pending" | "checker_unavailable" | "clock_uncertain" | "config_rollback" | "configuration_invalid" | "connection_refused" | "connection_timeout" | "cpu_pressure" | "csrf_invalid" | "deadline_exceeded" | "degraded" | "dependency_unavailable" | "disabled" | "disk_pressure" | "dispatch_fence_rejected" | "dns_failed" | "epoch_conflict" | "event_position_gap" | "event_publish_failed" | "event_quarantined" | "file_locked" | "healthy" | "historical" | "identity_mismatch" | "idle" | "initialization_required" | "invalid_report" | "invalid_request" | "keyring_invalid" | "loop_failed" | "manifest_mismatch" | "memory_pressure" | "not_configured" | "observer_offline" | "observer_reconnecting" | "observer_revoked" | "observer_scope_denied" | "observer_session_expired" | "ok" | "origin_denied" | "outbox_backpressure" | "payload_too_large" | "permission_denied" | "process_exited" | "process_locator_missing" | "process_not_found" | "protocol_unsupported" | "rate_limited" | "reload_error" | "report_identity_mismatch" | "report_invalid" | "report_stale" | "report_write_failed" | "resource_warning" | "response_limit_exceeded" | "response_mismatch" | "retired" | "schedule_delayed" | "sequence_conflict" | "snapshot_expired" | "stale_sequence" | "state_instance_mismatch" | "state_unavailable" | "state_version_unsupported" | "storage_unconfigured" | "target_not_found" | "target_removed" | "tls_invalid" | "ui_not_built" | "unavailable" | "unknown" | "unlimited" | "unsafe_file" | "vantage_mismatch";
            retired: boolean;
            target_id: string;
            /** @enum {string} */
            target_kind: "node" | "replica" | "endpoint" | "dependency";
            to_state: string;
        };
        Error: {
            meta: components["schemas"]["Meta"];
            error: {
                /** @enum {string} */
                code: "check_pending" | "not_configured" | "storage_unconfigured" | "process_locator_missing" | "process_not_found" | "process_exited" | "identity_mismatch" | "ambiguous_identity" | "connection_refused" | "connection_timeout" | "dns_failed" | "tls_invalid" | "authentication_failed" | "response_mismatch" | "permission_denied" | "adapter_unsupported" | "protocol_unsupported" | "vantage_mismatch" | "report_stale" | "report_invalid" | "report_identity_mismatch" | "clock_uncertain" | "dependency_unavailable" | "checker_unavailable" | "loop_failed" | "deadline_exceeded" | "observer_offline" | "observer_revoked" | "epoch_conflict" | "assignment_mismatch" | "schedule_delayed" | "adapter_blocked" | "adapter_error" | "response_limit_exceeded" | "cpu_pressure" | "memory_pressure" | "disk_pressure" | "unlimited" | "configuration_invalid" | "reload_error" | "auth_configuration_unavailable" | "keyring_invalid" | "state_unavailable" | "state_version_unsupported" | "ui_not_built" | "invalid_request" | "invalid_report" | "authentication_required" | "observer_session_expired" | "csrf_invalid" | "origin_denied" | "observer_scope_denied" | "target_not_found" | "target_removed" | "snapshot_expired" | "manifest_mismatch" | "stale_sequence" | "sequence_conflict" | "payload_too_large" | "rate_limited" | "initialization_required" | "resource_warning" | "historical" | "healthy" | "degraded" | "unavailable" | "unknown" | "disabled" | "idle" | "retired" | "observer_reconnecting" | "report_write_failed" | "file_locked" | "unsafe_file" | "state_instance_mismatch" | "config_rollback" | "ok" | "event_publish_failed" | "event_position_gap" | "event_quarantined" | "outbox_backpressure" | "dispatch_fence_rejected" | "broker_unavailable";
                message_key: string;
                /** Format: uuid */
                request_id: string;
                retry_after_ms: number;
            };
        };
        Self: {
            build_id: string;
            /** @constant */
            protocol_major: 1;
            uptime_ms: string;
            config_state: string;
            auth_state: {
                initialized: boolean;
                available: boolean;
                revision: string;
            };
            store_state: {
                status: string;
                budget: {
                    [key: string]: number;
                } | null;
            };
            collectors: {
                [key: string]: number | boolean;
            };
            limits: {
                concurrent_probes?: number;
                history_points?: number;
                max_nodes?: number;
                max_probes?: number;
                max_replicas?: number;
                probes_per_node?: number;
                store_max_bytes?: number;
            };
            retention: components["schemas"]["Retention"];
            warnings: string[];
            manifest_revision: string;
            server_boot_id: string;
            observers: components["schemas"]["NodeView"][];
            diagnostic_gaps: components["schemas"]["Check"][];
        };
        NodeViewPage: {
            items: components["schemas"]["NodeView"][];
            total: number;
            next_cursor: string;
            snapshot_id: string;
            /** Format: date-time */
            expires_at: string;
        };
        ComponentViewPage: {
            items: components["schemas"]["ComponentView"][];
            total: number;
            next_cursor: string;
            snapshot_id: string;
            /** Format: date-time */
            expires_at: string;
        };
        TransitionPage: {
            items: components["schemas"]["Transition"][];
            total: number;
            next_cursor: string;
            snapshot_id: string;
            /** Format: date-time */
            expires_at: string;
        };
        Empty: Record<string, never>;
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    diagnostics_get__session_bootstrap: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Bootstrap"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_post__session_login: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["LoginRequest"];
            };
        };
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Session"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__session: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Session"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_post__session_activity: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Empty"];
            };
        };
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Activity"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_post__session_logout: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["Empty"];
            };
        };
        responses: {
            /** @description Signed out; no response body */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__overview: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Overview"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__nodes: {
        parameters: {
            query?: {
                state?: string;
                kind?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["NodeViewPage"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__nodes__id_: {
        parameters: {
            query?: {
                snapshot_id?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["NodeDetail"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__components: {
        parameters: {
            query?: {
                state?: string;
                kind?: string;
                node_id?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["ComponentViewPage"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__components__id_: {
        parameters: {
            query?: {
                snapshot_id?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["ComponentDetail"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__replicas__id_: {
        parameters: {
            query?: {
                snapshot_id?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["ReplicaView"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__checks__id_: {
        parameters: {
            query?: {
                snapshot_id?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Check"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__transitions: {
        parameters: {
            query?: {
                target_kind?: string;
                target_id?: string;
                from?: string;
                to?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["TransitionPage"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__metrics: {
        parameters: {
            query: {
                target_kind: string;
                target_id: string;
                metric_key: string;
                dimension_id?: string;
                from: string;
                to: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["MetricSeries"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    diagnostics_get__self: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Independent diagnostic view; target failure remains HTTP 200 */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        meta: components["schemas"]["Meta"];
                        data: components["schemas"]["Self"];
                    };
                };
            };
            /** @description Safe error code */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    observer_sessions: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ObserverSessionRequest"];
            };
        };
        responses: {
            /** @description Durably accepted */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ObserverSession"];
                };
            };
            /** @description Safe error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    observer_reports: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ObserverFrame"];
            };
        };
        responses: {
            /** @description Durably accepted */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ObserverAck"];
                };
            };
            /** @description Safe error */
            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Error"];
                };
            };
        };
    };
    healthz: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Fixed public status only */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        /** @enum {string} */
                        status: "ok" | "ready" | "not_ready";
                    };
                };
            };
            /** @description Independent diagnostic service not ready */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
        };
    };
    readyz: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Fixed public status only */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        /** @enum {string} */
                        status: "ok" | "ready" | "not_ready";
                    };
                };
            };
            /** @description Independent diagnostic service not ready */
            503: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
        };
    };
}
