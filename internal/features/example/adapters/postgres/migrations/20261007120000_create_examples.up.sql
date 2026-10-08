CREATE TABLE examples (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT examples_name_key UNIQUE (name)
);
