# Security Policy (Chính Sách Bảo Mật)

The LLM Gateway team takes the security and privacy of our software, dependencies, and users seriously. This document outlines our policy for reporting security vulnerabilities, supported versions, and best practices for running a secure gateway instance.

---

## 🛡️ Supported Versions (Các Phiên Bản Được Hỗ Trợ)

We actively maintain and provide security patches for the following versions:

| Version | Supported | Security Patch Policy |
| :--- | :---: | :--- |
| `v0.4.x` | ✅ | Current Stable Release, all security patches |
| `v0.3.x` | ✅ | Critical vulnerabilities and high-severity bug fixes |
| `v0.2.x` | ❌ | End of Life (EOL) — Please upgrade to `v0.3.x` or later |
| `< v0.2.0` | ❌ | Unsupported |

---

## 🚨 Reporting a Vulnerability (Quy Trình Báo Cáo Lỗ Hổng)

**Please DO NOT report security vulnerabilities via public GitHub Issues, Discussions, or Pull Requests.** Publicly disclosing a vulnerability can endanger production systems before a fix is available.

### Preferred Reporting Method

1. **GitHub Private Vulnerability Reporting (Recommended)**:
   - Navigate to the **Security** tab of this repository on GitHub.
   - Click on **Report a vulnerability** to open a private draft advisory.
2. **Email Disclosure**:
   - If you cannot use GitHub Advisories, please email: **`tcc3281@gmail.com`**
   - Use the subject line: `[SECURITY] LLM Gateway - <Brief Description>`
   - If possible, encrypt or secure any sensitive proof-of-concept (PoC) code.

### Information to Include in Your Report

To help us investigate and patch the issue quickly, please provide:
- **Type of issue**: (e.g., Auth bypass, SQL injection, Prompt Injection escape, Sensitive data leakage, DoS / Resource exhaustion).
- **Affected component**: (e.g., `cmd/gateway`, `internal/plugin`, `internal/auth`, `web/src`, or Docker configuration).
- **Step-by-step reproduction**: Clear steps, scripts, or `curl` commands demonstrating the vulnerability.
- **Impact**: Potential consequences if exploited by an adversary.
- **Suggested remediation**: (Optional) Patch ideas or mitigation strategies if you have them.

---

## ⏱️ Response & Resolution Timeline

We strive to handle security reports with the highest priority:
- **Initial Response**: Within **48 hours**, acknowledging receipt of your report.
- **Triage & Assessment**: Within **5 business days**, confirming reproduction and assessing severity (CVSS score).
- **Patch & Advisory**: We will develop and test a fix in a private repository/branch, and release a security advisory along with a patched version (e.g., `v0.2.3` or `v0.3.1`).
- **Public Attribution**: Unless you request otherwise, we will credit your responsible disclosure in our release notes and GitHub Security Advisory.

---

## 🔒 Security Best Practices for Operators

When deploying LLM Gateway in production:
1. **Never commit secrets**: Keep `.env`, master keys, database passwords, and provider API tokens out of version control (use `.gitignore` and secret managers).
2. **Run with Docker & Non-Root user**: Always deploy using the provided container images which run without root privileges.
3. **Enable Redis & Postgres Authentication**: Ensure databases require strong passwords and operate within a private VPC / Docker bridge network.
4. **Use Built-in Security Plugins**:
   - Enable `rate-limiting` to prevent abuse and denial-of-service.
   - Enable `pii-masking` to sanitize personal data before forwarding to third-party LLM providers.
   - Enable `prompt-guard` to intercept prompt injection and jailbreak payloads.
5. **Enforce HTTPS / TLS**: Terminate TLS at a reverse proxy (e.g., Nginx, Cloudflare, Traefik) before traffic hits the Gateway.
