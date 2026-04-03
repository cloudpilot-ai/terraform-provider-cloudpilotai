# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly.

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please send an email to [security@cloudpilot.ai](mailto:security@cloudpilot.ai) with the following information:

- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact
- Any suggested fixes (optional)

We will acknowledge receipt of your report within **48 hours** and aim to provide a fix within **7 days** for critical issues.

## Security Best Practices

When using this Terraform provider:

1. **API Keys**: Never commit API keys to version control. Use environment variables or Terraform variables with sensitive flag.
2. **HTTPS Only**: The provider enforces HTTPS for all API communications. Do not attempt to use HTTP endpoints.
3. **Least Privilege**: Use API keys with the minimum required permissions.
4. **State Files**: Terraform state files may contain sensitive data. Always encrypt state files at rest and restrict access.
