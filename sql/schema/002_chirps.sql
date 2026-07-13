-- +goose Up
CREATE TABLE chirps
(
    id         uuid PRIMARY KEY,
    created_at TIMESTAMP                  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP                  NOT NULL DEFAULT NOW(),
    body       TEXT                       NOT NULL,
    user_id    uuid REFERENCES users (id) ON DELETE CASCADE NOT NULL
);

-- +goose Down
DROP TABLE chirps;