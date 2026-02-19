# Bedrock Protocol Formal Specification

Version: 0.2 Status: Research / Pre‑Formal Verification Draft

------------------------------------------------------------------------

# 1. Scope

This document extends the Bedrock protocol specification with:

-   Explicit safety and liveness invariants
-   Formalized state transition model
-   Slashing conditions
-   Validator set update rules
-   Execution determinism contract
-   Model checking outline (TLA+ style abstraction)

------------------------------------------------------------------------

# 2. Formal System Model

Let:

-   n = number of validators
-   f = maximum Byzantine validators
-   n ≥ 3f + 1

System is partially synchronous with unknown GST (Global Stabilization
Time).

------------------------------------------------------------------------

# 3. State Machine Definition

Global State S_h at height h consists of:

S_h = { BlockTree, LockedBlock, HighestQC, ValidatorSet,
ApplicationStateRoot }

Transition function:

S\_{h+1} = ApplyBlock(B_h, S_h)

Where ApplyBlock must be deterministic.

------------------------------------------------------------------------

# 4. Safety Invariants

Invariant 1 (Single Commit Rule):

For any height h, at most one block can be committed by honest
validators.

Invariant 2 (Lock Monotonicity):

If validator locks block B at height h, then it cannot vote for
conflicting block at same height unless justified by higher QC.

Invariant 3 (QC Intersection):

Any two quorum certificates must share at least f+1 honest validators.

Proof Sketch:

QC requires 2f+1 signatures. Two QCs imply overlap of at least
(2f+1)+(2f+1)-n ≥ f+1.

Since honest validators do not double-vote, conflicting QCs cannot form.

------------------------------------------------------------------------

# 5. Liveness Conditions

Assume:

-   After GST, message delay bounded by Δ.
-   Honest proposer selected infinitely often.
-   Timeout increases adaptively.

Then:

Within bounded rounds, QC forms and commit progresses.

------------------------------------------------------------------------

# 6. Slashing Conditions

A validator is slashable if:

1.  Double Proposal: Signs two different proposals at same (height,
    round).

2.  Double Vote: Signs two votes for different blocks at same (height,
    round).

3.  Surround Vote (if implemented): Vote violates locking rule by
    supporting conflicting chain.

Slashing Evidence:

Proof = { conflicting_message_1, conflicting_message_2,
validator_signature }

Evidence is verifiable cryptographically.

------------------------------------------------------------------------

# 7. Validator Set Updates

Validator set changes must occur at epoch boundaries.

Rules:

-   Update block must contain new validator set.
-   Update must be finalized before activation.
-   n_new ≥ 3f_new + 1 must hold.
-   Transition must preserve safety via joint consensus phase.

Joint Consensus Phase:

Validators_old ∪ Validators_new participate until overlap guarantees
safety.

------------------------------------------------------------------------

# 8. Deterministic Execution Contract

Execution environment must satisfy:

-   No wall clock access
-   No nondeterministic syscalls
-   No floating-point nondeterminism
-   Canonical serialization
-   Deterministic gas accounting

Execution function:

(state_root_new, receipts) = Execute(block, state_root_old)

All honest nodes must produce identical state_root_new.

------------------------------------------------------------------------

# 9. Fork Choice Rule

Fork choice selects chain with:

1.  Highest QC height
2.  Tie broken by highest round
3.  Deterministic hash comparison

Fork choice must be deterministic.

------------------------------------------------------------------------

# 10. Snapshot Integrity Guarantees

Snapshot at height h must satisfy:

-   Snapshot state_root == committed state_root
-   Snapshot hash verified via Merkle proof
-   Snapshot signed by ≥ 2f+1 validators (optional enhancement)

------------------------------------------------------------------------

# 11. Adversarial Scenarios

Analyzed Attacks:

-   Equivocation attack
-   Delayed vote attack
-   Network partition attack
-   Eclipse attempt
-   Proposal withholding

Security depends on:

-   Honest majority ≥ 2f+1
-   Proper peer scoring
-   Locking rule enforcement

------------------------------------------------------------------------

# 12. Formal Model Checking Outline (TLA+ Style)

Variables:

-   height
-   round
-   lockedBlock
-   highestQC
-   votes
-   proposals

Actions:

-   Propose
-   Vote
-   FormQC
-   Commit
-   Timeout
-   ViewChange

Safety Property:

\[\] ¬(Committed(b1) ∧ Committed(b2) ∧ b1 ≠ b2 ∧ height(b1)=height(b2))

Liveness Property:

◇ Committed(height = h+1)

Model constraints:

-   Byzantine nodes may send arbitrary messages.
-   Honest nodes follow protocol.

Model checking recommended for small n (4--7 validators).

------------------------------------------------------------------------

# 13. Performance Considerations

Complexity:

-   Vote verification: O(n)
-   QC aggregation (BLS optional): O(1) verification
-   Gossip bandwidth: O(n\^2) worst-case without optimization

Optimizations:

-   Aggregated BLS signatures
-   Vote compression
-   Pipelined rounds
-   Threshold signatures

------------------------------------------------------------------------

# 14. Parameter Constraints

Minimum validator set: 4 Recommended production: ≥ 7

QC threshold: 2f + 1

Timeout base: configurable (e.g., 2 seconds)

Block size: bounded by deterministic execution limits.

------------------------------------------------------------------------

# 15. Open Research Areas

-   Formal proof in Coq or Isabelle
-   Full TLA+ model
-   Adaptive adversary resistance
-   Validator churn safety proof
-   Cryptographic aggregation optimization

------------------------------------------------------------------------

# 16. Conformance Requirements

An implementation is compliant if:

-   It preserves Safety Invariants 1--3.
-   It produces identical state roots across honest nodes.
-   It prevents double voting via locking.
-   It tolerates ≤ f Byzantine validators without conflicting commit.

------------------------------------------------------------------------

End of Formal Specification v0.2
