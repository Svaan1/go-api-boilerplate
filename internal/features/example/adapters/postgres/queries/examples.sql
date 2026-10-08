-- name: CreateExample :exec
INSERT INTO examples (id, name, created_at, updated_at)
VALUES ($1, $2, $3, $4);

-- name: GetExampleForUpdate :one
SELECT id, name, created_at, updated_at
FROM examples
WHERE id = $1
FOR UPDATE;

-- name: UpdateExample :execrows
UPDATE examples
SET name = $2, updated_at = $3
WHERE id = $1;
