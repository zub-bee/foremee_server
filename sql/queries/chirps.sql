-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, body, user_id)
VALUES (
            gen_random_uuid(),
            now(),
            now(),
            $1,
            $2
       )
RETURNING *;

-- name: GetAllChirps :many
SELECT id, created_at, updated_at, body, user_id
FROM chirps
ORDER BY created_at;

-- name: GetOneChirp :one
SELECT id, created_at, updated_at, body, user_id
FROM chirps
WHERE id = $1;