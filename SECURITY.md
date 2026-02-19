# Security Policy

Bedrock is a safety-critical distributed systems project. Security issues are treated as high priority.

This document explains how to report vulnerabilities, what is in scope, and how coordinated disclosure works.

---

## Supported Versions

Bedrock may be used in different modes (research, testnet, production hardening). Unless this repository explicitly publishes releases, **only the default branch (`main`) is considered supported**.

If releases/tags exist, the maintainers will define support windows here. Example policy (recommended):

- **Latest stable release**: supported
- **Previous stable release**: security fixes only (time-limited)
- **All older releases**: not supported

If you are unsure whether a version is supported, report anyway.

---

## Reporting a Vulnerability

**Do not open public GitHub issues or pull requests for security problems.**

Instead, report privately with enough detail to reproduce and assess impact.

### Preferred Reporting Channel

- **GitHub Security Advisories** (private report): use the “Report a vulnerability” button in the repository’s *Security* tab (if enabled).

### Alternative (Email)

If GitHub private reporting is not available:

- Email the maintainers at: **security@aweh.com** *(replace with your project’s real address)*

Include “Bedrock Security” in the subject.

### What to Include

Please provide:

- A clear description of the vulnerability
- Affected component(s): consensus / networking / mempool / execution / storage / sync / infra
- Impact assessment (what breaks: safety, liveness, funds, privacy, DoS, integrity)
- Reproduction steps or proof-of-concept (PoC)
- Logs, configs, and environment details (OS, architecture, commit hash)
- Whether the issue is known to be exploited in the wild (if you have evidence)
- Suggested fix or mitigation (optional)

If the report is incomplete, we may ask follow-ups—but we will still treat the report as sensitive.

---

## Coordinated Disclosure

Bedrock follows coordinated vulnerability disclosure (CVD):

1. **Acknowledgement**: We aim to acknowledge receipt within **72 hours**.
2. **Triage**: We assess severity, affected scope, and exploitability.
3. **Fix Development**: We develop and test a patch privately when feasible.
4. **Release**: We publish a fix (patch, release, or advisory).
5. **Public Disclosure**: We disclose details after users have reasonable time to upgrade.

### Embargo Window

Typical embargo windows by severity (guidance, not a promise):

- **Critical**: 7–14 days (as fast as safely possible)
- **High**: 14–30 days
- **Medium/Low**: 30–90 days

We may shorten or extend based on real-world risk, complexity, and coordination needs.

---

## Severity Guidelines

Because Bedrock is a BFT protocol node, severity is evaluated differently than a typical web service.

### Critical

- Breaks **safety** (e.g., conflicting commits possible with ≤ 1/3 Byzantine)
- Allows **state root divergence** among honest nodes
- Enables **remote compromise** of nodes
- Enables large-scale **network eclipse** leading to safety compromise
- Causes consensus to finalize invalid state transitions

### High

- Breaks **liveness** under realistic network conditions
- Enables **DoS** that can halt finality on testnet/mainnet configurations
- Allows **validator equivocation** to go undetected or unpenalized
- Subverts snapshot sync integrity (e.g., accepting invalid snapshots)

### Medium / Low

- Increases attack surface without immediate exploit
- Minor information disclosure
- Local-only issues with limited impact

---

## Scope

### In Scope

- Consensus engine (proposer/validator logic, QC handling, view change, locking rules)
- Signature verification (BLS/Ed25519), hashing, and serialization
- Deterministic execution (WASM/Wasmtime), state transitions, state root commitments
- Snapshot sync (integrity, verification, chunking, replay protection)
- P2P networking (discovery, gossip, scoring, rate limiting, anti-eclipse)
- Mempool policy (validation, ordering, eviction, spam resistance)
- Storage integrity (RocksDB usage, corruption handling, crash consistency)
- CI/CD pipeline and release artifacts (if published)

### Out of Scope

- Issues in third-party dependencies without a Bedrock-specific exploit path (please still report if you think Bedrock usage makes it exploitable)
- Social engineering attacks
- Physical attacks on infrastructure
- Denial-of-service that requires unrealistic resources and does not affect real deployments

If you are uncertain, report it.

---

## Safe Harbor

We support good-faith security research intended to improve Bedrock’s security.

We will not pursue legal action against researchers who:

- Follow this policy
- Avoid privacy violations, data destruction, and service disruption
- Give us reasonable time to remediate before public disclosure

This safe harbor does not apply to actions that violate applicable law or cause harm.

---

## Dependency Security

Bedrock depends on third-party libraries (e.g., libp2p, Wasmtime, RocksDB). If a dependency issue impacts Bedrock, please:

- Include dependency name and version
- Include how Bedrock uses it
- Provide a Bedrock-specific reproduction when possible

We may address issues by upgrading dependencies, adding mitigations, or restricting unsafe functionality.

---

## Security Best Practices for Operators

If you run Bedrock nodes:

- Use pinned versions (tags/commits) and verify checksums for artifacts
- Run nodes with least privilege (non-root user)
- Restrict RPC endpoints behind firewalls/VPN
- Enable rate limits on ingress
- Monitor consensus health (finality latency, fork signals, peer churn, bandwidth)
- Keep keys in secure storage (HSM/KMS if applicable)
- Rotate credentials and peer identities when compromised
- Treat snapshot sources as untrusted; verify all proofs and roots

---

## Vulnerability Disclosure Credits

We are happy to credit reporters in release notes/advisories **if requested** and if it is safe to do so.

Please indicate how you would like to be credited (name, handle, link).

---

## PGP (Optional)

If you prefer encrypted email, publish a PGP public key and fingerprint here.

- PGP Key: *(TBD)*
- Fingerprint: *(TBD)*
