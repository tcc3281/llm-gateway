---
name: code-review-solid
description: >-
  Scans a codebase (or a specific file/package) for violations of the SOLID
  principles and missing/misapplied design patterns (Creational, Structural,
  Behavioral), then produces a structured report AND proposes concrete code
  revisions. Trigger this skill whenever the user asks to "review the code for
  design patterns", "check SOLID principles", "refactor for better
  architecture", "suggest design patterns", "audit the codebase", "is this
  code well-structured?", or anything that implies architectural quality
  analysis. Works for any language; generates pseudocode examples when the
  language is mixed or unspecified, and language-specific code when the
  language is clear from context.
---

# Code Review: SOLID Principles & Design Patterns

This skill guides a thorough architectural review of a codebase (or a scoped
subset of it). The review is split into two phases:

1. **SOLID Audit** — identify violations of each of the five principles.
2. **Design Pattern Opportunities** — identify missing or misapplied Creational,
   Structural, and Behavioral patterns.

For every issue found, produce both a diagnosis AND a concrete code suggestion.

---

## Phase 1 — Scan & Scope

Before writing any findings, orient yourself:

1. **Identify the target scope.** If the user named a specific file, package, or
   module, focus there. Otherwise, scan the top-level directory structure first
   to understand the project's architecture (layers, packages, modules).

2. **Detect the language(s).** Look for file extensions, build files
   (`go.mod`, `package.json`, `pyproject.toml`, `pom.xml`, etc.), or imports.
   If the project is polyglot, note each language.

3. **Identify architectural patterns already in use.** Note things like
   layered architecture (handler → service → repository), dependency injection
   containers, existing interfaces/abstractions, etc. This prevents false
   positives — don't flag something as a violation if the project intentionally
   uses a different but valid pattern.

4. **Prioritize your read list.** Start with files that are most likely to show
   structural issues: large files (>300 lines), files with many imports, files
   named `util`, `manager`, `helper`, or `god` (classic SRP red flags), and
   interface/abstract definitions.

---

## Phase 2 — SOLID Audit

Go through each principle in order. For each one, report:

- ✅ **OK** — no notable issues found (keep it brief)
- ⚠️ **Violation** — describe what is wrong, cite the file + line if possible,
  and provide a before/after code revision

### S — Single Responsibility Principle
A class/struct/module should have **one reason to change**. Red flags:
- A type that handles HTTP, business logic, AND database access at once
- A file/module over ~400 lines with unrelated concerns mixed together
- Method names from wildly different domains on the same type (e.g., `SendEmail` and `ValidateSchema` on a `UserService`)

### O — Open/Closed Principle
Entities should be **open for extension, closed for modification**. Red flags:
- Long `if-else` or `switch` chains that grow every time a new "type" is added
  (instead of a polymorphic dispatch)
- Adding a new provider/strategy forces edits to core business logic
- No use of interfaces/abstract types where behavior needs to vary

### L — Liskov Substitution Principle
Subtypes must be **drop-in replacements** for their base types. Red flags:
- A subtype implements a method by panicking, returning an error that the base
  never returns, or silently doing nothing
- An interface method is only implemented by some concrete types but forced on all
- Type assertions / `.(ConcreteType)` casts after accepting an interface
  (suggests the abstraction is leaky)

### I — Interface Segregation Principle
Prefer **many small, focused interfaces** over one fat one. Red flags:
- An interface with 10+ methods where callers only ever use 2–3
- A concrete type forced to implement methods with empty bodies or `panic("not supported")`
- Interfaces defined at the implementation site rather than the consumer site

### D — Dependency Inversion Principle
High-level modules depend on **abstractions, not concretions**. Red flags:
- A service or handler directly instantiating a database client, HTTP client, or
  third-party SDK inside its own constructor or method body
- No dependency injection — dependencies are hardcoded
- Importing low-level packages from high-level business logic files

---

## Phase 3 — Design Pattern Opportunities

After the SOLID audit, scan for places where a well-known pattern would
simplify the code. Focus on patterns that give the highest leverage — don't
force every pattern onto the code just for completeness. Be honest about
tradeoffs.

### Creational
| Pattern | When to suggest |
|---|---|
| **Factory Method** | Multiple `if/switch` blocks that construct different concrete types based on a discriminator string/enum |
| **Abstract Factory** | Families of related objects that must be created together (e.g., different DB drivers + their query builders) |
| **Builder** | Constructors with many optional parameters, or structs initialized in many scattered places with partial field sets |
| **Prototype** | Expensive-to-initialize objects created repeatedly with minor variations |
| **Singleton** | Shared stateful resources (DB pool, logger, config) constructed more than once |

### Structural
| Pattern | When to suggest |
|---|---|
| **Adapter** | Calling a third-party library whose interface doesn't match what the rest of the app expects |
| **Bridge** | Two independent dimensions of variation combined into an explosion of subclasses |
| **Composite** | Recursive tree structures (menus, policies, permissions) treated inconsistently |
| **Decorator** | Cross-cutting concerns (logging, caching, retries, auth) added by copy-pasting into methods |
| **Facade** | Complex subsystem accessed from many places with repeated multi-step setup |
| **Flyweight** | Large numbers of fine-grained objects sharing significant state |
| **Proxy** | Lazy loading, access control, or caching wrapped around an existing interface |

### Behavioral
| Pattern | When to suggest |
|---|---|
| **Chain of Responsibility** | Sequential middleware/handler chains built with ad-hoc if/else instead of a pipeline |
| **Command** | Operations that need to be queued, logged, or undone |
| **Observer** | One object's state change needs to notify many others — currently done with direct calls or global variables |
| **Strategy** | Algorithms or behaviors selected at runtime via conditionals instead of interchangeable objects |
| **State** | An object's behavior changes based on internal state managed with a tangle of booleans/enums |
| **Template Method** | Multiple similar algorithms sharing a skeleton but differing in a few steps, currently duplicated |
| **Mediator** | Many objects communicate directly with each other, creating tight coupling |

---

## Phase 4 — Output Format

Structure your response as a single **Code Review Report** using this template
(adapt sections to what was actually found — skip "Creational" if no issues there, etc.):

```
# Code Review Report: SOLID & Design Patterns
## Scope
[What was scanned, language(s) detected, existing architectural patterns noted]

## SOLID Audit

### ✅/⚠️ S — Single Responsibility
[Finding + file reference]
**Before:**
[code snippet]
**After:**
[revised code snippet with explanation]

### ✅/⚠️ O — Open/Closed
...

### ✅/⚠️ L — Liskov Substitution
...

### ✅/⚠️ I — Interface Segregation
...

### ✅/⚠️ D — Dependency Inversion
...

## Design Pattern Opportunities

### [Pattern Name] in [File/Package]
**Why:** [one-sentence justification]
**Before:**
[code snippet]
**After:**
[revised code snippet]

## Summary Table
| # | Principle/Pattern | Severity | File | Status |
|---|---|---|---|---|
| 1 | SRP | 🔴 High | internal/service/user.go | Proposed fix above |

## Recommended Next Steps
[Ordered list — most impactful changes first]
```

### Code snippet language rules
- Use the project's **actual language** for before/after snippets when you know it.
- If the project is polyglot or you're writing a general example, use **pseudocode**
  (label it with ` ```pseudocode `).
- Keep snippets **focused** — show only the relevant lines + just enough context
  to understand the change. Don't paste entire files.
- Annotate the diff: use `// ✅ added`, `// ❌ removed`, or `// ℹ️ reason` comments.

### Severity levels
Use these consistently in the summary table:
- 🔴 **High** — the violation actively hurts extensibility, testability, or causes bugs
- 🟡 **Medium** — a code smell that will cause pain as the project grows
- 🟢 **Low** — a minor improvement; fine to skip if team disagrees

---

## Tips for Good Reviews

- **Be constructive, not prescriptive.** Explain *why* a change matters, not just
  that a rule was broken. The goal is to help the developer understand, not just comply.
- **Respect existing patterns.** If the codebase already uses a framework that
  handles DI or routing, don't flag its conventions as violations.
- **Don't over-pattern.** A simple script doesn't need the Strategy pattern.
  Only suggest patterns where the complexity clearly justifies them.
- **Propose PRs, not rewrites.** The revision snippets should be surgically
  minimal — the smallest change that fixes the issue.
- **Pseudocode is first-class.** When the exact syntax isn't the point, write
  clear pseudocode that any developer can translate to their language.
