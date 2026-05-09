#!/usr/bin/env python3
"""Local E2E test runner for the Cannectors test lab (story 23.3).

Reads scenario YAML files under test-lab/scenarios/, resets the lab between
scenarios, runs the referenced pipeline once, then evaluates assertions
(HTTP request journal, SQL queries, pipeline log).

Usage:
  test-lab/run.py                       # run every scenario
  test-lab/run.py auth-input-bearer     # run scenarios whose name matches any arg
  SCENARIO=retry test-lab/run.py        # filter via env var (substring match)

Exit code: 0 if every scenario passes, 1 otherwise.
"""
from __future__ import annotations

import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

REPO_ROOT = Path(__file__).resolve().parent.parent
SCENARIOS_DIR = REPO_ROOT / "test-lab" / "scenarios"
SCRIPTS_DIR = REPO_ROOT / "test-lab" / "scripts"
STATE_DIR = REPO_ROOT / "test-lab" / "state"
WIREMOCK_BASE = os.environ.get("WIREMOCK_BASE", "http://localhost:18080")
COMPOSE_FILE = REPO_ROOT / "test-lab" / "docker-compose.yml"

ANSI_GREEN = "\033[32m"
ANSI_RED = "\033[31m"
ANSI_YELLOW = "\033[33m"
ANSI_DIM = "\033[2m"
ANSI_RESET = "\033[0m"


def green(s: str) -> str:
    return f"{ANSI_GREEN}{s}{ANSI_RESET}" if sys.stdout.isatty() else s


def red(s: str) -> str:
    return f"{ANSI_RED}{s}{ANSI_RESET}" if sys.stdout.isatty() else s


def yellow(s: str) -> str:
    return f"{ANSI_YELLOW}{s}{ANSI_RESET}" if sys.stdout.isatty() else s


def dim(s: str) -> str:
    return f"{ANSI_DIM}{s}{ANSI_RESET}" if sys.stdout.isatty() else s


@dataclass
class AssertionResult:
    label: str
    ok: bool
    detail: str = ""


@dataclass
class ScenarioResult:
    name: str
    path: Path
    pipeline: str
    pipeline_status: str = ""
    expected_status: str = "success"
    assertions: list[AssertionResult] = field(default_factory=list)
    log_path: Path | None = None
    error: str = ""

    @property
    def ok(self) -> bool:
        if self.error:
            return False
        if self.pipeline_status != self.expected_status:
            return False
        return all(a.ok for a in self.assertions)


# ---------- lab helpers ------------------------------------------------------


def http_get(path: str) -> dict[str, Any]:
    out = subprocess.run(
        ["curl", "-fsS", f"{WIREMOCK_BASE}{path}"],
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    return json.loads(out) if out.strip() else {}


def http_delete(path: str) -> None:
    subprocess.run(
        ["curl", "-fsS", "-X", "DELETE", f"{WIREMOCK_BASE}{path}"],
        check=True,
        capture_output=True,
    )


def http_post_empty(path: str) -> None:
    subprocess.run(
        ["curl", "-fsS", "-X", "POST", f"{WIREMOCK_BASE}{path}"],
        check=True,
        capture_output=True,
    )


def reset_journal() -> None:
    http_delete("/__admin/requests")


def reset_scenarios() -> None:
    http_post_empty("/__admin/scenarios/reset")


def reset_mappings() -> None:
    http_post_empty("/__admin/mappings/reset")


def reset_state_dir() -> None:
    if not STATE_DIR.exists():
        return
    for f in STATE_DIR.glob("state-*.json"):
        f.unlink()


def reset_database() -> None:
    reset_sql = (REPO_ROOT / "test-lab" / "postgres" / "reset.sql").read_text()
    seed_sql = (REPO_ROOT / "test-lab" / "postgres" / "init" / "002_seed.sql").read_text()
    proc = subprocess.run(
        [
            "docker",
            "compose",
            "-f",
            str(COMPOSE_FILE),
            "exec",
            "-T",
            "postgres",
            "psql",
            "-U",
            "cannectors_test",
            "-d",
            "cannectors_test",
        ],
        input=reset_sql + "\n" + seed_sql,
        text=True,
        capture_output=True,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"db reset failed: {proc.stderr}")


def psql_query(query: str) -> str:
    """Run a SQL query and return the single-cell result as a stripped string."""
    proc = subprocess.run(
        [
            "docker",
            "compose",
            "-f",
            str(COMPOSE_FILE),
            "exec",
            "-T",
            "postgres",
            "psql",
            "-U",
            "cannectors_test",
            "-d",
            "cannectors_test",
            "-At",
            "-c",
            query,
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"psql query failed: {proc.stderr.strip()}")
    return proc.stdout.strip()


# ---------- pipeline execution ----------------------------------------------


def run_pipeline_once(pipeline: Path, timeout: int = 30) -> Path:
    """Run a pipeline via the existing helper script and return the log file path."""
    log_file = Path(
        subprocess.run(
            ["mktemp"], capture_output=True, text=True, check=True
        ).stdout.strip()
    )
    helper = SCRIPTS_DIR / "run-pipeline-once.sh"
    with log_file.open("w") as f:
        subprocess.run(
            ["bash", str(helper), str(pipeline), str(timeout)],
            stdout=f,
            stderr=subprocess.STDOUT,
            check=False,
        )
    return log_file


def parse_pipeline_status(log_file: Path, pipeline_id: str) -> str:
    status = ""
    with log_file.open() as f:
        for line in f:
            if '"msg":"execution completed"' not in line:
                continue
            try:
                evt = json.loads(line)
            except json.JSONDecodeError:
                continue
            if evt.get("pipeline_id") == pipeline_id:
                status = evt.get("status", "")
    return status


# ---------- assertion engine -------------------------------------------------


def request_matches(req: dict[str, Any], spec: dict[str, Any]) -> bool:
    rq = req.get("request", {})
    rd = req.get("responseDefinition", {})
    method = spec.get("method")
    if method and rq.get("method") != method:
        return False
    path = spec.get("path")
    if path is not None:
        actual_path = (rq.get("url") or "").split("?")[0]
        if actual_path != path:
            return False
    path_prefix = spec.get("path_prefix")
    if path_prefix is not None:
        actual_path = (rq.get("url") or "").split("?")[0]
        if not actual_path.startswith(path_prefix):
            return False
    path_regex = spec.get("path_regex")
    if path_regex is not None:
        actual_path = (rq.get("url") or "").split("?")[0]
        if not re.search(path_regex, actual_path):
            return False
    status = spec.get("status")
    if status is not None and rd.get("status") != status:
        return False
    headers = spec.get("headers") or {}
    req_headers = rq.get("headers") or {}
    # WireMock returns headers with their on-wire casing; do a
    # case-insensitive lookup.
    req_headers_ci = {k.lower(): v for k, v in req_headers.items()}
    for k, v in headers.items():
        if req_headers_ci.get(k.lower()) != v:
            return False
    query = spec.get("query") or {}
    req_query = rq.get("queryParams") or {}
    for k, v in query.items():
        actual = (req_query.get(k) or {}).get("values") or []
        if not actual or actual[0] != v:
            return False
    body_contains = spec.get("body_contains")
    if body_contains is not None:
        if body_contains not in (rq.get("body") or ""):
            return False
    return True


def count_requests(spec: dict[str, Any]) -> int:
    journal = http_get("/__admin/requests")
    return sum(1 for r in journal.get("requests", []) if request_matches(r, spec))


def evaluate_assertion(idx: int, spec: dict[str, Any], log_file: Path) -> AssertionResult:
    if "http_count_eq" in spec:
        params = spec["http_count_eq"]
        expected = int(params["expected"])
        match_params = {k: v for k, v in params.items() if k != "expected"}
        actual = count_requests(match_params)
        return AssertionResult(
            label=f"http_count_eq {match_params}",
            ok=actual == expected,
            detail=f"expected={expected}, actual={actual}",
        )
    if "http_count_ge" in spec:
        params = spec["http_count_ge"]
        expected = int(params["expected"])
        match_params = {k: v for k, v in params.items() if k != "expected"}
        actual = count_requests(match_params)
        return AssertionResult(
            label=f"http_count_ge {match_params}",
            ok=actual >= expected,
            detail=f"expected>={expected}, actual={actual}",
        )
    if "http_count_eq_zero" in spec:
        params = spec["http_count_eq_zero"]
        actual = count_requests(params)
        return AssertionResult(
            label=f"http_count_eq_zero {params}",
            ok=actual == 0,
            detail=f"expected=0, actual={actual}",
        )
    if "sql_eq" in spec:
        params = spec["sql_eq"]
        query = params["query"]
        expected = str(params["expected"])
        actual = psql_query(query)
        return AssertionResult(
            label=f"sql_eq {shlex.quote(query)}",
            ok=actual == expected,
            detail=f"expected={expected!r}, actual={actual!r}",
        )
    if "log_contains" in spec:
        needle = spec["log_contains"]
        text = log_file.read_text()
        return AssertionResult(
            label=f"log_contains {needle!r}",
            ok=needle in text,
            detail="" if needle in text else "needle not found in log",
        )
    if "log_not_contains" in spec:
        needle = spec["log_not_contains"]
        text = log_file.read_text()
        return AssertionResult(
            label=f"log_not_contains {needle!r}",
            ok=needle not in text,
            detail="" if needle not in text else "needle unexpectedly found in log",
        )
    return AssertionResult(
        label=f"assertion #{idx}",
        ok=False,
        detail=f"unknown assertion type: {spec!r}",
    )


# ---------- scenario lifecycle ----------------------------------------------


def load_scenario(path: Path) -> dict[str, Any]:
    with path.open() as f:
        data = yaml.safe_load(f) or {}
    if "name" not in data:
        data["name"] = path.stem
    if "pipeline" not in data:
        raise ValueError(f"scenario {path}: missing required 'pipeline' field")
    return data


def apply_setup(setup: dict[str, Any]) -> None:
    # Defaults: every scenario starts from a clean lab. The order matters:
    # reload mappings first, reset WireMock state, then clear the journal.
    if setup.get("reset_mappings", True):
        reset_mappings()
    if setup.get("reset_scenarios", True):
        reset_scenarios()
    if setup.get("reset_journal", True):
        reset_journal()
    if setup.get("reset_state", True):
        reset_state_dir()
    if setup.get("reset_db", True):
        reset_database()
    for cmd in setup.get("commands", []):
        subprocess.run(cmd, shell=True, check=True)


def run_scenario(path: Path) -> ScenarioResult:
    spec = load_scenario(path)
    result = ScenarioResult(
        name=spec["name"],
        path=path,
        pipeline=spec["pipeline"],
        expected_status=spec.get("expect_status", "success"),
    )
    print(f"\n{yellow('==')} {spec['name']} {dim('(' + str(spec.get('pipeline')) + ')')}")

    try:
        apply_setup(spec.get("setup", {}))
    except Exception as exc:  # noqa: BLE001
        result.error = f"setup failed: {exc}"
        print(red(f"  setup failed: {exc}"))
        return result

    pipeline_path = REPO_ROOT / spec["pipeline"]
    if not pipeline_path.exists():
        result.error = f"pipeline not found: {pipeline_path}"
        print(red(f"  {result.error}"))
        return result

    pipeline_id = yaml.safe_load(pipeline_path.read_text())["name"]
    timeout = int(spec.get("timeout", 30))

    log_file = run_pipeline_once(pipeline_path, timeout=timeout)
    result.log_path = log_file
    result.pipeline_status = parse_pipeline_status(log_file, pipeline_id)

    status_ok = result.pipeline_status == result.expected_status
    status_marker = green("ok") if status_ok else red("FAIL")
    print(
        f"  {status_marker} pipeline status={result.pipeline_status or '<unknown>'} "
        f"(expected={result.expected_status})"
    )

    for idx, assertion in enumerate(spec.get("assertions", [])):
        ar = evaluate_assertion(idx, assertion, log_file)
        result.assertions.append(ar)
        marker = green("ok") if ar.ok else red("FAIL")
        suffix = f"  {dim(ar.detail)}" if ar.detail else ""
        print(f"  {marker} {ar.label}{suffix}")

    return result


def discover_scenarios(filters: list[str]) -> list[Path]:
    if not SCENARIOS_DIR.exists():
        raise SystemExit(f"scenarios dir not found: {SCENARIOS_DIR}")
    scenarios = sorted(SCENARIOS_DIR.glob("*.yaml"))
    if not filters:
        return scenarios
    matched: list[Path] = []
    for f in filters:
        for s in scenarios:
            if f in s.stem and s not in matched:
                matched.append(s)
    return matched


def print_summary(results: list[ScenarioResult]) -> None:
    print("\n" + "=" * 70)
    total = len(results)
    failed = [r for r in results if not r.ok]
    if not failed:
        print(green(f"all {total} scenarios passed"))
        return
    print(red(f"{len(failed)} of {total} scenarios FAILED"))
    for r in failed:
        print(f"\n--- {red(r.name)} ({r.path.relative_to(REPO_ROOT)})")
        if r.error:
            print(f"    error: {r.error}")
        if r.pipeline_status != r.expected_status:
            print(
                f"    pipeline status={r.pipeline_status!r} "
                f"(expected={r.expected_status!r})"
            )
        for a in r.assertions:
            if not a.ok:
                print(f"    FAIL {a.label}  {a.detail}")
        if r.log_path and r.log_path.exists():
            tail = r.log_path.read_text().splitlines()[-15:]
            print("    --- last 15 log lines ---")
            for line in tail:
                print(f"    {line}")


def main(argv: list[str]) -> int:
    if not shutil.which("docker"):
        print(red("docker is required to run the test lab"))
        return 1

    filters: list[str] = list(argv[1:])
    env_filter = os.environ.get("SCENARIO", "").strip()
    if env_filter:
        filters.extend(env_filter.split(","))

    scenarios = discover_scenarios([f for f in filters if f])
    if not scenarios:
        print(red("no scenarios matched"))
        return 1

    print(f"running {len(scenarios)} scenario(s)")
    started = time.time()
    results = [run_scenario(s) for s in scenarios]
    elapsed = time.time() - started

    print_summary(results)
    print(f"\nelapsed: {elapsed:.1f}s")

    for r in results:
        if r.log_path and r.log_path.exists():
            r.log_path.unlink(missing_ok=True)

    return 0 if all(r.ok for r in results) else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
