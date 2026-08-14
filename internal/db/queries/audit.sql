-- name: InsertAuditLog :exec
INSERT INTO audit_log (
    actor_type, actor_id, client_id, action, resource_type, resource_id, ip, user_agent, metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
);
