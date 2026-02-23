---
name: cyberark-pam
description: "Manage CyberArk Privileged Access Manager (PAM) via REST API and PVWA. Accounts, safes, credentials retrieval, PSM sessions, platforms, users, and web application connectors."
metadata: {"nanobot":{"emoji":"🔐","requires":{"bins":["curl"]}}}
---

# CyberArk PAM Skill

Manage CyberArk Privileged Access Manager (PAM Self-Hosted) using the PVWA REST API. All API calls go through HTTPS to the PVWA server. Every request (except Logon) requires an `Authorization` header with a session token.

Docs: https://docs.cyberark.com/pam-self-hosted/latest/en/content/webservices/implementing%20privileged%20account%20security%20web%20services%20.htm

## Authentication

```bash
# CyberArk authentication (returns session token)
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/auth/CyberArk/Logon" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"P@ssw0rd"}' | jq -r '.'

# LDAP authentication
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/auth/LDAP/Logon" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"P@ssw0rd"}' | jq -r '.'

# RADIUS authentication
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/auth/RADIUS/Logon" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"P@ssw0rd"}' | jq -r '.'

# Store token for subsequent calls
TOKEN=$(curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/auth/CyberArk/Logon" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"P@ssw0rd"}' | jq -r '.')

# Logoff (invalidate session)
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/auth/Logoff" \
  -H "Authorization: $TOKEN"
```

## Accounts

```bash
# List accounts (with optional search)
curl -s -k "https://PVWA_HOST/PasswordVault/API/Accounts?search=webserver&limit=25" \
  -H "Authorization: $TOKEN" | jq '.value[]'

# List accounts filtered by safe
curl -s -k "https://PVWA_HOST/PasswordVault/API/Accounts?filter=safeName%20eq%20ProductionSafe" \
  -H "Authorization: $TOKEN" | jq '.value[]'

# Get specific account details
curl -s -k "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID" \
  -H "Authorization: $TOKEN" | jq '.'

# Retrieve password (credential retrieval)
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID/Password/Retrieve" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reason":"Automated deployment"}' | jq -r '.'

# Add a new account
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Accounts" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Operating System-WebServer-10.0.1.10-root",
    "address": "10.0.1.10",
    "userName": "root",
    "platformId": "UnixSSH",
    "safeName": "ProductionSafe",
    "secretType": "password",
    "secret": "InitialP@ssw0rd",
    "platformAccountProperties": {
      "LogonDomain": "",
      "Port": "22"
    },
    "secretManagement": {
      "automaticManagementEnabled": true
    }
  }' | jq '.'

# Update account (partial update)
curl -s -k -X PATCH "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '[{"op":"replace","path":"/address","value":"10.0.1.20"}]'

# Delete account
curl -s -k -X DELETE "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID" \
  -H "Authorization: $TOKEN"

# Change password (initiate CPM password change)
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID/Change" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"ChangeEntireGroup": false}'

# Verify password (CPM verification)
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID/Verify" \
  -H "Authorization: $TOKEN"

# Reconcile password
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Accounts/ACCOUNT_ID/Reconcile" \
  -H "Authorization: $TOKEN"
```

## Safes

```bash
# List all safes
curl -s -k "https://PVWA_HOST/PasswordVault/API/Safes" \
  -H "Authorization: $TOKEN" | jq '.value[]'

# Search safes
curl -s -k "https://PVWA_HOST/PasswordVault/API/Safes?search=Production" \
  -H "Authorization: $TOKEN" | jq '.value[]'

# Get safe details
curl -s -k "https://PVWA_HOST/PasswordVault/API/Safes/ProductionSafe" \
  -H "Authorization: $TOKEN" | jq '.'

# Create a safe
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Safes" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "safeName": "NewProductionSafe",
    "description": "Safe for production server credentials",
    "managingCPM": "PasswordManager",
    "numberOfDaysRetention": 30
  }' | jq '.'

# Update safe
curl -s -k -X PUT "https://PVWA_HOST/PasswordVault/API/Safes/ProductionSafe" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "safeName": "ProductionSafe",
    "description": "Updated description",
    "numberOfDaysRetention": 60
  }'

# Delete safe
curl -s -k -X DELETE "https://PVWA_HOST/PasswordVault/API/Safes/ProductionSafe" \
  -H "Authorization: $TOKEN"

# List safe members
curl -s -k "https://PVWA_HOST/PasswordVault/API/Safes/ProductionSafe/Members" \
  -H "Authorization: $TOKEN" | jq '.value[]'

# Add safe member
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Safes/ProductionSafe/Members" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "memberName": "DevOpsTeam",
    "memberType": "Group",
    "permissions": {
      "useAccounts": true,
      "retrieveAccounts": true,
      "listAccounts": true,
      "viewAuditLog": true,
      "viewSafeMembers": true
    }
  }'
```

## Platforms

```bash
# List all platforms
curl -s -k "https://PVWA_HOST/PasswordVault/API/Platforms" \
  -H "Authorization: $TOKEN" | jq '.Platforms[]'

# Get platform details
curl -s -k "https://PVWA_HOST/PasswordVault/API/Platforms/UnixSSH" \
  -H "Authorization: $TOKEN" | jq '.'

# Activate a platform
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Platforms/PLATFORM_ID/Activate" \
  -H "Authorization: $TOKEN"

# Deactivate a platform
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Platforms/PLATFORM_ID/Deactivate" \
  -H "Authorization: $TOKEN"
```

## Users

```bash
# List users
curl -s -k "https://PVWA_HOST/PasswordVault/API/Users" \
  -H "Authorization: $TOKEN" | jq '.Users[]'

# Search users
curl -s -k "https://PVWA_HOST/PasswordVault/API/Users?search=admin" \
  -H "Authorization: $TOKEN" | jq '.Users[]'

# Get user details
curl -s -k "https://PVWA_HOST/PasswordVault/API/Users/USER_ID" \
  -H "Authorization: $TOKEN" | jq '.'

# Create user
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/Users" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "svc_deploy",
    "initialPassword": "Temp@1234",
    "userType": "EPVUser",
    "enableUser": true,
    "changePassOnNextLogon": true
  }' | jq '.'

# Delete user
curl -s -k -X DELETE "https://PVWA_HOST/PasswordVault/API/Users/USER_ID" \
  -H "Authorization: $TOKEN"
```

## PSM Sessions & Monitoring

```bash
# List live sessions
curl -s -k "https://PVWA_HOST/PasswordVault/API/LiveSessions" \
  -H "Authorization: $TOKEN" | jq '.LiveSessions[]'

# List recorded sessions
curl -s -k "https://PVWA_HOST/PasswordVault/API/Recordings?limit=25" \
  -H "Authorization: $TOKEN" | jq '.Recordings[]'

# Terminate a live session
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/LiveSessions/SESSION_ID/Terminate" \
  -H "Authorization: $TOKEN"

# Get PSM server details
curl -s -k "https://PVWA_HOST/PasswordVault/API/PSM/Servers" \
  -H "Authorization: $TOKEN" | jq '.'
```

## PSM Web Application Connectors

PSM supports automated web application login via connection components. Key concepts:

### WebForm Fields Syntax

WebForm fields define how PSM interacts with the web login page. Each row in the WebFormFields list specifies a DOM interaction:

**Input field** (type text into an element):
```
Username > {Username} (searchby=id)
Password > {Password} (searchby=id)
```

**Click button:**
```
submit-button > (Button) (searchby=id)
```

**ScriptClick** (JavaScript click, for elements standard click doesn't work on):
```
submit-button > (ScriptClick) (searchby=id)
```

**Validation** (wait for element to appear, confirms successful login):
```
dashboard-header > (Validation) (searchby=id)
```

**Frame** (switch to an iframe before interacting):
```
login-frame > (Frame) (searchby=id)
```

**Redirect** (navigate to a URL before login):
```
https://myapp.example.com/login > (Redirect)
```

**Failure** (detect login failure):
```
error-message > (Failure) (searchby=class)
```

### Search By Options

- `searchby=id` — HTML element ID
- `searchby=name` — HTML element name attribute
- `searchby=class` — CSS class name
- `searchby=xpath` — XPath expression
- `searchby=css` — CSS selector
- `searchby=linktext` — Anchor text

### Placeholders

- `{Username}` — Account username from CyberArk
- `{Password}` — Account password from CyberArk
- `{Address}` — Target address from account properties
- `{LogonDomain}` — Logon domain from account properties
- `&parameter&` — PreConnect DLL output parameter

### WebApp Connection Component Setup

Key PVWA configuration paths for a web connector:
- **Connection Components** > **Target Settings** > **ClientDispatcher**: `{PSMComponentsFolder}\CyberArk.PSM.WebAppDispatcher.exe`
- **Connection Components** > **Target Settings** > **ClientApp**: `Chrome` or `Edge`
- **Connection Components** > **Target Settings** > **WebFormFields**: The login sequence
- **Connection Components** > **Target Settings** > **Client Specific** > **AllowedURLs**: Restrict navigation

### Browser Configuration

Supported browsers for PSM web connectors:
- Google Chrome 100+ (32-bit or 64-bit)
- Microsoft Edge 103+ (32-bit or 64-bit)

The browser driver version must match the browser version. Use `WebDriverUpgrader` for automatic updates.

### Example: Full Web App Connector Flow

```
# WebFormFields for a typical web login:
https://myapp.example.com/login > (Redirect)
login-frame > (Frame) (searchby=id)
username > {Username} (searchby=id)
password > {Password} (searchby=id)
loginButton > (Button) (searchby=id)
error-alert > (Failure) (searchby=class)
dashboard > (Validation) (searchby=id)
```

### PreConnect Custom Code

Run custom logic before the login process (e.g., generate temp user, MFA token):

1. Create a DLL implementing `IPreconnectContract` from `PreconnectUtils.dll`
2. Place DLL in `PSM\Components` folder
3. Configure in PVWA:
   - `PreConnectDllName`: Your DLL filename
   - `PreConnectParameters`: Comma-separated parameter names (e.g., `username,password`)
4. Use returned values in WebFormFields with `&parameter&` syntax

### EnableAdvancedDebugging

Enable visual debugging to capture screenshots of each WebForm step:

Set `EnableAdvancedDebugging = Yes` on the platform. Screenshots are saved to a folder showing each step of the automation. Delete the folder after debugging — it may contain sensitive data.

## System Health

```bash
# Check system health
curl -s -k "https://PVWA_HOST/PasswordVault/API/ComponentsMonitoringDetails/SessionManagement" \
  -H "Authorization: $TOKEN" | jq '.'

# Server info
curl -s -k "https://PVWA_HOST/PasswordVault/API/Server" \
  -H "Authorization: $TOKEN" | jq '.'
```

## Onboarding Rules

```bash
# List onboarding rules
curl -s -k "https://PVWA_HOST/PasswordVault/API/AutomaticOnboardingRules" \
  -H "Authorization: $TOKEN" | jq '.AutomaticOnboardingRules[]'

# Create onboarding rule
curl -s -k -X POST "https://PVWA_HOST/PasswordVault/API/AutomaticOnboardingRules" \
  -H "Authorization: $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "targetPlatformId": "UnixSSH",
    "targetSafeName": "DiscoveredAccounts",
    "isAdminIDFilter": false,
    "machineTypeFilter": "Server",
    "systemTypeFilter": "Unix"
  }'
```

## Swagger

Access the full interactive API docs at:
```
https://PVWA_HOST/PasswordVault/swagger/docs/v1
```

## Tips

- **Always use `-k`** with curl if the PVWA uses a self-signed certificate. In production, use proper CA certs.
- **Token expiry**: Sessions expire after inactivity (default 20 min). Re-authenticate if you get 401.
- **Rate limiting**: API returns 429 if too many requests. Implement backoff in scripts.
- **Search filter syntax**: Use URL-encoded OData filters: `filter=safeName%20eq%20MySafe`
- **PATCH operations**: Use JSON Patch format `[{"op":"replace","path":"/field","value":"new"}]`
- **PUT replaces entire resource** — missing fields become null. Always include all fields.
- **Audit trail**: All API calls are logged in the CyberArk audit. Use meaningful `reason` fields.
- **PACLI alternative**: If a REST API endpoint doesn't exist for your task, use the PACLI CLI.
- **Swagger UI**: Use `/PasswordVault/swagger` for interactive API exploration and testing.
