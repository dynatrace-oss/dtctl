# Security Policy

## Reporting a Vulnerability

If you believe you have found a security vulnerability in dtctl, please report it to us as described below.

### How to Report

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to opensource@dynatrace.com (or your preferred security contact email).

If you prefer to encrypt your report, PGP keys are available upon request.

Please include the requested information listed below (as much as you can provide) to help us better understand the nature and scope of the possible issue:

- Type of issue (e.g., buffer overflow, SQL injection, cross-site scripting, etc.)
- Full paths of source file(s) related to the manifestation of the issue
- The location of the affected source code (tag/branch/commit or direct URL)
- Any special configuration required to reproduce the issue
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit the issue

This information will help us triage your report more quickly.

### What to Report

**Please report (examples):**
- Potential security vulnerabilities in dtctl
- Vulnerabilities in dtctl dependencies

**Please do not report (examples):**
- Requests for security feature enhancements
- Assistance with security-related configuration
- Issues unrelated to security

### Response Process

We will acknowledge receipt of your vulnerability report and send you regular updates about our progress.

The Dynatrace Open Source Community and the bug submitter will negotiate a public disclosure date. We prefer to fully remediate the bug before public disclosure, which may take time depending on complexity.

**Typical disclosure timeline:**
- Immediate if vulnerability is already publicly known
- Within 7 days for issues with straightforward mitigation
- Several weeks for complex issues requiring significant code changes

### Confidentiality

All vulnerability information will be kept confidential within the dtctl maintainers and Dynatrace Open Source Community, except as necessary for remediation.

## Security Best Practices for Users

When using dtctl, follow these security best practices:

1. **Token Storage**: dtctl stores API tokens securely. Never commit config files containing tokens to version control.

2. **File Permissions**: Config files are created with restrictive permissions (0600 for files, 0700 for directories). Do not change these permissions.

3. **Network Security**: dtctl communicates exclusively over HTTPS with Dynatrace environments. Verify SSL certificates are valid.

4. **Editor Integration**: When using `dtctl edit`, ensure your EDITOR environment variable points to a trusted executable.

5. **Untrusted Input**: Be cautious when executing DQL queries or workflows from untrusted sources.

6. **Updates**: Keep dtctl updated to the latest version to receive security patches.

## Current Security Status

**Security Features Implemented** (as of 2026-01-09):

- ✅ **Secure Token Storage**: Tokens are stored in the OS keyring (macOS Keychain, Linux Secret Service, Windows Credential Manager) instead of plaintext config files
- ✅ **Input Validation**: Editor paths and file operations are validated to prevent command injection
- ✅ **Dependency Scanning**: Automated vulnerability scanning with `govulncheck` in CI/CD pipeline

To migrate existing plaintext tokens to secure storage:
```bash
dtctl config migrate-tokens
```

---

This security policy is based on the [Dynatrace Open Source Security Policy](https://github.com/dynatrace-oss).
