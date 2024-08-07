-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id UUID PRIMARY KEY,
    login VARCHAR(255) UNIQUE NOT NULL,
    password TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE NOT NULL,
    order_number VARCHAR NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'NEW',
    accrual NUMERIC(15, 2) NOT NULL DEFAULT 0.00 ,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE balance (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE NOT NULL,
    current NUMERIC(15, 2) NOT NULL DEFAULT 0.00 CHECK (current >= 0.00),
    withdrawn NUMERIC(15, 2) NOT NULL DEFAULT 0.00 CHECK (withdrawn >= 0.00),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE withdrawal (
    id UUID PRIMARY KEY,
    order_number VARCHAR NOT NULL UNIQUE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE NOT NULL,
    sum DOUBLE PRECISION NOT NULL DEFAULT 0.0 CHECK (sum >= 0.0),
    processed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE withdrawal;
DROP TABLE balance;
DROP TABLE orders;
DROP TABLE users;
-- +goose StatementEnd
