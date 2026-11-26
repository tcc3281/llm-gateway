#!/usr/bin/env python3
import json
import os
import re

workspace = "/data/per-pro/llm-gateway/skills/code-review-solid-workspace/iteration-1"

evals = [
    {
        "dir": "eval-0-srp-gateway",
        "name": "Eval 0: Gateway Service (SRP)",
        "assertions": [
            {
                "id": "covers_all_solid_principles",
                "text": "Evaluates all 5 SOLID principles (S, O, L, I, D)",
                "check": lambda text: all(re.search(p, text, re.IGNORECASE) for p in [
                    r"\b(single responsibility|SRP)\b",
                    r"\b(open/?closed|OCP)\b",
                    r"\b(liskov|LSP)\b",
                    r"\b(interface segregation|ISP)\b",
                    r"\b(dependency inversion|DIP)\b"
                ])
            },
            {
                "id": "flags_srp_violation",
                "text": "Explicitly flags Single Responsibility Principle (SRP) violation",
                "check": lambda text: bool(re.search(r"single responsibility|SRP", text, re.IGNORECASE) and re.search(r"violation|god object|responsibilit|multiple concerns", text, re.IGNORECASE))
            },
            {
                "id": "proposes_design_pattern",
                "text": "Recommends architectural design patterns (e.g. Facade, Strategy, Pipeline)",
                "check": lambda text: bool(re.search(r"strategy|pipeline|chain of responsibility|facade|builder|decorator", text, re.IGNORECASE))
            },
            {
                "id": "contains_before_after_code",
                "text": "Includes before/after refactoring code snippets",
                "check": lambda text: ("```" in text) and bool(re.search(r"before|after|proposed|refactor", text, re.IGNORECASE))
            },
            {
                "id": "includes_structured_summary",
                "text": "Provides structured summary table with findings and severities",
                "check": lambda text: "|" in text and ("Severity" in text or "Status" in text or "Principle" in text)
            }
        ]
    },
    {
        "dir": "eval-1-ocp-provider",
        "name": "Eval 1: Provider Extensibility (OCP)",
        "assertions": [
            {
                "id": "identifies_ocp_issue",
                "text": "Identifies Open/Closed Principle (OCP) violation for provider additions",
                "check": lambda text: bool(re.search(r"open/?closed|OCP", text, re.IGNORECASE) and re.search(r"provider|extensib|shotgun|switch", text, re.IGNORECASE))
            },
            {
                "id": "suggests_factory_or_registry",
                "text": "Suggests Factory Method, Abstract Factory, or Registry pattern",
                "check": lambda text: bool(re.search(r"factory|registry", text, re.IGNORECASE))
            },
            {
                "id": "provides_concrete_code_example",
                "text": "Provides concrete code snippets or pseudocode for provider abstraction",
                "check": lambda text: "```" in text and bool(re.search(r"type Provider|interface|struct", text, re.IGNORECASE))
            },
            {
                "id": "discusses_interface_abstraction",
                "text": "Discusses unified provider interface/adapter contract",
                "check": lambda text: bool(re.search(r"adapter|interface|contract|abstraction", text, re.IGNORECASE))
            },
            {
                "id": "includes_structured_summary",
                "text": "Provides a structured summary table of findings",
                "check": lambda text: "|" in text and ("Severity" in text or "Status" in text or "File" in text)
            }
        ]
    },
    {
        "dir": "eval-2-isp-dip-plugin",
        "name": "Eval 2: Plugin Architecture (ISP & DIP)",
        "assertions": [
            {
                "id": "evaluates_isp_and_dip",
                "text": "Explicitly evaluates Interface Segregation (ISP) and Dependency Inversion (DIP)",
                "check": lambda text: bool(re.search(r"interface segregation|ISP", text, re.IGNORECASE)) and bool(re.search(r"dependency inversion|DIP", text, re.IGNORECASE))
            },
            {
                "id": "suggests_behavioral_or_structural_pattern",
                "text": "Recommends relevant patterns (Chain of Responsibility, Pipeline, Decorator)",
                "check": lambda text: bool(re.search(r"chain of responsibility|pipeline|decorator|observer|interceptor", text, re.IGNORECASE))
            },
            {
                "id": "provides_refactoring_code",
                "text": "Provides concrete before/after code snippets or pseudocode",
                "check": lambda text: "```" in text and bool(re.search(r"before|after|proposed|refactor", text, re.IGNORECASE))
            },
            {
                "id": "addresses_concrete_dependencies",
                "text": "Highlights coupling to concrete database/transport types and provides decoupling",
                "check": lambda text: bool(re.search(r"Redis|Postgres|ResponseWriter|concrete", text, re.IGNORECASE) and re.search(r"interface|abstract|decoupl|inject", text, re.IGNORECASE))
            },
            {
                "id": "includes_structured_summary",
                "text": "Provides a structured summary table with severity ratings",
                "check": lambda text: "|" in text and ("Severity" in text or "Status" in text or "Principle" in text)
            }
        ]
    }
]

benchmark_data = {
    "skill_name": "code-review-solid",
    "iteration": 1,
    "evals": []
}

total_assertions_with = 0
passed_assertions_with = 0
total_assertions_without = 0
passed_assertions_without = 0

for ev in evals:
    eval_dir = os.path.join(workspace, ev["dir"])
    eval_res = {
        "id": ev["dir"],
        "name": ev["name"],
        "with_skill": {},
        "without_skill": {}
    }
    
    for variant in ["with_skill", "without_skill"]:
        review_path = os.path.join(eval_dir, variant, "outputs", "review.md")
        timing_path = os.path.join(eval_dir, variant, "timing.json")
        grading_path = os.path.join(eval_dir, variant, "grading.json")
        
        content = ""
        if os.path.exists(review_path):
            with open(review_path, "r", encoding="utf-8") as f:
                content = f.read()
                
        timing = {"total_duration_seconds": 0}
        if os.path.exists(timing_path):
            with open(timing_path, "r", encoding="utf-8") as f:
                timing = json.load(f)
                
        expectations = []
        variant_passed = 0
        for assertion in ev["assertions"]:
            passed = assertion["check"](content) if content else False
            if passed:
                variant_passed += 1
            evidence = f"Verified pattern match in {variant}/outputs/review.md (length: {len(content)} chars)" if passed else "Condition not met in generated output."
            expectations.append({
                "text": assertion["text"],
                "passed": passed,
                "evidence": evidence
            })
            
        with open(grading_path, "w", encoding="utf-8") as f:
            json.dump({"expectations": expectations}, f, indent=2)
            
        pass_rate = (variant_passed / len(ev["assertions"])) * 100 if ev["assertions"] else 0
        eval_res[variant] = {
            "passed": variant_passed,
            "total": len(ev["assertions"]),
            "pass_rate": pass_rate,
            "duration_seconds": timing.get("total_duration_seconds", 0),
            "output_chars": len(content)
        }
        
        if variant == "with_skill":
            passed_assertions_with += variant_passed
            total_assertions_with += len(ev["assertions"])
        else:
            passed_assertions_without += variant_passed
            total_assertions_without += len(ev["assertions"])
            
    benchmark_data["evals"].append(eval_res)

overall_with_pass_rate = (passed_assertions_with / total_assertions_with) * 100
overall_without_pass_rate = (passed_assertions_without / total_assertions_without) * 100

benchmark_data["summary"] = {
    "with_skill": {
        "passed": passed_assertions_with,
        "total": total_assertions_with,
        "pass_rate": overall_with_pass_rate
    },
    "without_skill": {
        "passed": passed_assertions_without,
        "total": total_assertions_without,
        "pass_rate": overall_without_pass_rate
    }
}

with open(os.path.join(workspace, "benchmark.json"), "w", encoding="utf-8") as f:
    json.dump(benchmark_data, f, indent=2)

# Generate benchmark.md
md_lines = [
    "# Benchmark Results: `code-review-solid` (Iteration 1)",
    "",
    "## Overall Performance",
    "",
    f"| Metric | With Skill (`code-review-solid`) | Baseline (No Skill) | Delta |",
    f"| :--- | :--- | :--- | :--- |",
    f"| **Assertion Pass Rate** | **{overall_with_pass_rate:.1f}%** ({passed_assertions_with}/{total_assertions_with}) | **{overall_without_pass_rate:.1f}%** ({passed_assertions_without}/{total_assertions_without}) | **+{overall_with_pass_rate - overall_without_pass_rate:.1f}%** |",
    "",
    "## Per-Evaluation Breakdown",
    "",
    "| Test Case | With Skill Pass Rate | Baseline Pass Rate | With Skill Duration | Baseline Duration |",
    "| :--- | :--- | :--- | :--- | :--- |"
]

for ev in benchmark_data["evals"]:
    md_lines.append(
        f"| **{ev['name']}** | {ev['with_skill']['pass_rate']:.0f}% ({ev['with_skill']['passed']}/{ev['with_skill']['total']}) | {ev['without_skill']['pass_rate']:.0f}% ({ev['without_skill']['passed']}/{ev['without_skill']['total']}) | {ev['with_skill']['duration_seconds']:.1f}s | {ev['without_skill']['duration_seconds']:.1f}s |"
    )

with open(os.path.join(workspace, "benchmark.md"), "w", encoding="utf-8") as f:
    f.write("\n".join(md_lines))

print("Grading & Benchmark aggregation complete!")
