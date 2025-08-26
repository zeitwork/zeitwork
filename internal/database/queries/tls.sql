-- name: TLSCertificateFindById :one
SELECT * FROM tls_certificates WHERE id = $1;

-- name: TLSCertificateFindByDomain :one
SELECT * FROM tls_certificates WHERE domain = $1;

-- name: TLSCertificateFindExpiring :many
SELECT * FROM tls_certificates 
WHERE expires_at < (NOW() + INTERVAL '30 days') AND auto_renew = true
ORDER BY expires_at;

-- name: TLSCertificateFind :many
SELECT * FROM tls_certificates ORDER BY domain;

-- name: TLSCertificateCreate :one
INSERT INTO tls_certificates (domain, certificate, private_key, issuer, expires_at, auto_renew) 
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: TLSCertificateUpdate :one
UPDATE tls_certificates 
SET certificate = $2, private_key = $3, issuer = $4, expires_at = $5, updated_at = NOW() 
WHERE id = $1 RETURNING *;

-- name: TLSCertificateDelete :exec
DELETE FROM tls_certificates WHERE id = $1;

-- name: TLSCertificateToggleAutoRenew :one
UPDATE tls_certificates 
SET auto_renew = $2, updated_at = NOW() 
WHERE id = $1 RETURNING *;
