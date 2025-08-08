# Auth Providers Configuration File Format

The authentication providers configuration file can be in JSON or YAML format. It defines the settings for various authentication methods used in the application. Below is the structure of the configuration file:

## Structure

```yaml
anonymous:
  allowed: true
  role: "user"

api_keys:
  my_api_key:
    key: "your_api_key_here"
    header: "x-hugr-api-key"
    default_role: "admin"
    headers:
      role: "x-hugr-role"
      user_id: "x-hugr-user-id"
      user_name: "x-hugr-user-name"

jwt:
  my_jwt_provider:
    issuer: "your_issuer_here"
    public_key: "your_public_key_here"
    cookie_name: "your_cookie_name_here"
    scope_role_prefix: "hugr:"
    role_header: "x-hugr-role"
    claims:
      role: "x-hugr-role"
      user_id: "sub"
      user_name: "name"

redirect_login_paths:
  - "/login"
  - "/auth"

managed_api_keys: true
login_url: "/login"
redirect_url: "/home"
secret_key: "your_secret_key_here"
```

## Fields

- **anonymous**: Configuration for anonymous authentication.
  - **allowed**: Boolean indicating if anonymous access is allowed.
  - **role**: Role assigned to anonymous users.

- **api_keys**: A map of API key configurations.
  - **key**: The actual API key.
  - **header**: The header name to look for the API key.
  - **default_role**: The default role assigned if no role is provided.
  - **headers**: Configuration for additional headers.
    - **role**: Header name for the role.
    - **user_id**: Header name for the user ID.
    - **user_name**: Header name for the user name.

- **jwt**: A map of JWT provider configurations.
  - **issuer**: The issuer of the JWT.
  - **public_key**: The public key used to verify the JWT.
  - **cookie_name**: The name of the cookie to extract the JWT from.
  - **scope_role_prefix**: Prefix used for role scopes.
  - **role_header**: Header name for the role if not found in claims.
  - **claims**: Configuration for claims mapping.
    - **role**: Claim key for the role.
    - **user_id**: Claim key for the user ID.
    - **user_name**: Claim key for the user name.
  
- **oidc**: OpenID Connect provider configuration (not shown in the example but can be added similarly).
  - **issuer**: The issuer URL for the OIDC provider.
  - **client_id**: The client ID for the OIDC application.
  - **timeout**: The timeout duration for OIDC requests.
  - **tls_insecure**: Boolean indicating if TLS verification should be skipped.
  - **cookie_name**: The name of the cookie to store the OIDC token.
  - **scope_role_prefix**: Prefix used for role scopes in OIDC.
  - **claims**: Configuration for claims mapping in OIDC.
    - **role**: Claim key for the role.
    - **user_id**: Claim key for the user ID.
    - **user_name**: Claim key for the user name.

- **managed_api_keys**: Boolean indicating if API keys are managed by the application.
- **redirect_login_paths**: List of paths to redirect to for login.

- **login_url**: URL for the login page.
- **redirect_url**: URL to redirect to after successful authentication.

- **secret_key**: API Key that should be provided in the header X-Hugr-Secret to access the API with admin role. The default headers to identify the user are:
  - X-Hugr-User-Id
  - X-Hugr-User-Name
  - X-Hugr-Role
