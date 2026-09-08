# Security Policy

## Supported Versions

Evey is a self-hosted personal media server. Only the latest version receives security fixes.

| Version | Supported |
|---------|-----------|
| latest  | ✓         |
| older   | ✗         |

## Reporting a Vulnerability

**Do not open a public issue for security vulnerabilities.**

Report via:
- **GitHub Private Advisory**: https://github.com/evey-project/evey/security/advisories/new
- **Email**: security@eveyproject.org

Include: description, reproduction steps, impact assessment, and any suggested fix.

### Response Timeline

| Stage | Target |
|-------|--------|
| Acknowledgement | 48 hours |
| Status update | 7 days |
| Fix (critical) | 30 days |
| Fix (non-critical) | 90 days |

## Out of Scope

- Issues requiring physical access to the server
- Denial-of-service attacks
- Vulnerabilities in third-party dependencies (report upstream)
- Self-inflicted misconfiguration (e.g., exposing admin port publicly)
