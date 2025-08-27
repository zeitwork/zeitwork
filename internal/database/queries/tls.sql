-- name: TlsCertificateCreate :one
INSERT INTO tls_certificates (domain, certificate, private_key, expires_at, issuer, auto_renew)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: TlsCertificateList :many
SELECT * FROM tls_certificates ORDER BY domain;

-- name: TlsCertificateFindByDomain :one
SELECT * FROM tls_certificates WHERE domain = $1;

-- name: TlsCertificateUpdate :one
UPDATE tls_certificates 
SET certificate = $2, private_key = $3, expires_at = $4, updated_at = NOW()
WHERE domain = $1
RETURNING *;

-- name: TlsCertificateDelete :exec
DELETE FROM tls_certificates WHERE domain = $1;