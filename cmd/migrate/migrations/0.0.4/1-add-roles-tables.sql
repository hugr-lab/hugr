CREATE TABLE roles (
    name VARCHAR NOT NULL PRIMARY KEY,
    description VARCHAR,
    disabled BOOLEAN NOT NULL DEFAULT FALSE
);

INSERT INTO roles (name, description)
VALUES ('admin', 'Admin role'), ('public', 'Public role'), ('readonly', 'Readonly role');

CREATE TABLE permissions (
    role VARCHAR NOT NULL,
    type_name VARCHAR NOT NULL,
    field_name VARCHAR NOT NULL,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    filter JSON,
    data JSON,
    PRIMARY KEY (role, type_name, field_name),
);

INSERT INTO permissions (role, type_name, field_name, hidden, disabled)
VALUES
    ('readonly', 'Mutation', '*', false, true);