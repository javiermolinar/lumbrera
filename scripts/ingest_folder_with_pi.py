#!/usr/bin/env python3
"""Stage local files as Markdown and ask pi's Lumbrera ingest skill to ingest them.

Use this when you manually export/copy Google Docs or other documents into a
local folder and want an agent to do semantic Lumbrera ingestion: preserve the
source, search for overlap, choose tags/summaries, create/update wiki pages, and
verify.

The script does not create wiki pages itself. It stages each input file under
.brain/imports/local/ and invokes pi with /skill:lumbrera-ingest.

Supported input by default:
  - .md, .markdown, .mdx: copied as Markdown
  - .txt: wrapped as Markdown
  - .html, .htm, .docx: converted to GitHub-flavored Markdown with pandoc

Example:
  python scripts/ingest_folder_with_pi.py \
    --brain ~/brains/tempo \
    --input-dir ~/Downloads/tempo-docs

If your skill is named /skill:ingest instead:
  python scripts/ingest_folder_with_pi.py \
    --brain ~/brains/tempo \
    --input-dir ~/Downloads/tempo-docs \
    --skill-name ingest
"""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import json
import os
import re
import shlex
import shutil
import signal
import subprocess
import sys
import threading
import time
from dataclasses import dataclass
from pathlib import Path

DEFAULT_EXTENSIONS = {".md", ".markdown", ".mdx", ".txt", ".html", ".htm", ".docx"}
PANDOC_EXTENSIONS = {".html", ".htm", ".docx"}


@dataclass(frozen=True)
class LocalDoc:
    input_path: Path
    relative_path: Path
    staged_rel: str
    source_rel: str
    title: str


def main() -> int:
    args = parse_args()
    brain = Path(args.brain).expanduser().resolve()
    input_dir = Path(args.input_dir).expanduser().resolve()
    if not brain.exists():
        print(f"brain path does not exist: {brain}", file=sys.stderr)
        return 2
    if not input_dir.is_dir():
        print(f"input directory does not exist or is not a directory: {input_dir}", file=sys.stderr)
        return 2

    docs = collect_docs(input_dir, args)
    if not docs:
        print("No supported files found.")
        return 0
    print(f"Found {len(docs)} supported file(s) under {input_dir}")

    staged = 0
    skipped = 0
    failed = 0
    for index, doc in enumerate(docs, start=1):
        source_abs = brain / doc.source_rel
        if source_abs.exists():
            message = f"source already exists: {doc.source_rel}"
            if args.on_exists == "skip":
                print(f"SKIP {doc.relative_path}: {message}")
                skipped += 1
                continue
            print(f"FAIL {doc.relative_path}: {message}", file=sys.stderr)
            failed += 1
            continue

        started = time.monotonic()
        print(f"\n[{index}/{len(docs)}] Staging {doc.relative_path} -> {doc.staged_rel}")
        prompt = agent_prompt(brain, input_dir, doc, args)
        if args.dry_run:
            print(prompt)
            staged += 1
            continue

        try:
            print(f"[{index}/{len(docs)}] Converting to Markdown")
            markdown = file_to_markdown(doc.input_path, doc.title)
            staged_abs = brain / doc.staged_rel
            staged_abs.parent.mkdir(parents=True, exist_ok=True)
            staged_abs.write_text(markdown, encoding="utf-8")
            print(f"[{index}/{len(docs)}] Wrote staged Markdown: {doc.staged_rel} ({len(markdown)} bytes)")
            print(f"[{index}/{len(docs)}] Starting pi ingest for target source: {doc.source_rel}")
            run_pi_ingest(brain, prompt, args)
            if not source_abs.exists():
                raise RuntimeError(f"pi exited without preserving expected source: {doc.source_rel}")
            print(f"[{index}/{len(docs)}] Confirmed source exists: {doc.source_rel}")
            print(f"[{index}/{len(docs)}] Finished in {time.monotonic() - started:.1f}s")
            staged += 1
        except Exception as exc:  # noqa: BLE001 - CLI should report and continue.
            print(f"FAIL {doc.relative_path}: {exc}", file=sys.stderr)
            failed += 1
            if args.stop_on_error:
                break

    print(f"Done. handed_to_agent={staged} skipped={skipped} failed={failed}")
    return 1 if failed else 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Stage local documents and call pi /skill:lumbrera-ingest.")
    parser.add_argument("--brain", required=True, help="Target Lumbrera brain directory. pi runs with this as cwd.")
    parser.add_argument("--input-dir", required=True, help="Folder containing exported/copied documents.")
    parser.add_argument("--recursive", action=argparse.BooleanOptionalAction, default=True, help="Recursively scan --input-dir. Default: true.")
    parser.add_argument("--extension", action="append", default=[], help="Allowed file extension, e.g. .md. Repeatable. Defaults to md/markdown/mdx/txt/html/htm/docx.")
    parser.add_argument("--include", action="append", default=[], help="Relative glob to include, e.g. 'sources/tempo/traceql/**'. Repeatable. Defaults to all supported files.")
    parser.add_argument("--exclude", action="append", default=[], help="Relative glob to exclude, e.g. '**/_index.md'. Repeatable.")
    parser.add_argument("--pi", default="pi --no-session --no-extensions --thinking medium", help="pi command. Default disables sessions/extensions and uses medium thinking for batch ingestion while keeping skills enabled.")
    parser.add_argument("--pi-arg", action="append", default=[], help="Extra argument passed to pi before the prompt. Repeatable.")
    parser.add_argument("--pi-timeout", type=float, default=900, help="Per-document pi timeout in seconds. Default: 900. Use 0 to disable.")
    parser.add_argument("--pi-json-log", action=argparse.BooleanOptionalAction, default=True, help="Run pi in JSON event mode and print tool/message progress. Default: true.")
    parser.add_argument("--skill-name", default="lumbrera-ingest", help="pi skill command name. Use 'ingest' for /skill:ingest.")
    parser.add_argument("--stage-dir", default=".brain/imports/local", help="Repo-relative staging directory ignored by Lumbrera verification.")
    parser.add_argument("--source-prefix", default="sources/imports", help="Lumbrera source path prefix for preserved Markdown.")
    parser.add_argument("--actor", default="folder-importer", help="Actor label to ask the agent to use for lumbrera write.")
    parser.add_argument("--reason-prefix", default="Import local document", help="Reason prefix to ask the agent to use for source preservation.")
    parser.add_argument("--curation-policy", choices=["default", "canonical", "historical", "source-only"], default="default", help="Semantic ingestion policy passed to the agent. Use canonical for current product docs, historical for old design docs, source-only to preserve without wiki synthesis.")
    parser.add_argument("--versioned", action="store_true", help="Include input file mtime in source/staged filenames. Useful because Lumbrera sources are immutable.")
    parser.add_argument("--on-exists", choices=["skip", "fail"], default="skip", help="What to do when the target Lumbrera source path already exists.")
    parser.add_argument("--agent-dry-run", action="store_true", help="Stage Markdown and print the pi prompt, but do not call pi.")
    parser.add_argument("--dry-run", action="store_true", help="Print planned prompts without staging or calling pi.")
    parser.add_argument("--stop-on-error", action="store_true", help="Stop after the first failed document.")
    args = parser.parse_args()

    args.extensions = normalize_extensions(args.extension)
    validate_repo_relative_dir(parser, args.stage_dir, "--stage-dir")
    validate_repo_relative_dir(parser, args.source_prefix, "--source-prefix")
    if not args.source_prefix.startswith("sources/"):
        parser.error("--source-prefix must be under sources/")
    return args


def normalize_extensions(values: list[str]) -> set[str]:
    if not values:
        return DEFAULT_EXTENSIONS
    out = set()
    for value in values:
        value = value.strip().lower()
        if not value:
            continue
        if not value.startswith("."):
            value = "." + value
        out.add(value)
    return out


def validate_repo_relative_dir(parser: argparse.ArgumentParser, value: str, flag: str) -> None:
    path = Path(value)
    if path.is_absolute() or ".." in path.parts:
        parser.error(f"{flag} must be repo-relative and must not contain ..")


def collect_docs(input_dir: Path, args: argparse.Namespace) -> list[LocalDoc]:
    pattern = "**/*" if args.recursive else "*"
    docs = []
    for path in sorted(input_dir.glob(pattern)):
        if not path.is_file() or path.name.startswith("."):
            continue
        if path.suffix.lower() not in args.extensions:
            continue
        rel = path.relative_to(input_dir)
        rel_posix = rel.as_posix()
        if args.include and not any(glob_matches(rel_posix, pattern) for pattern in args.include):
            continue
        if args.exclude and any(glob_matches(rel_posix, pattern) for pattern in args.exclude):
            continue
        source_rel = source_path_for_file(path, rel, args)
        staged_rel = staged_path_for_file(path, rel, args)
        docs.append(LocalDoc(path, rel, staged_rel, source_rel, title_for_path(path)))
    return docs


def glob_matches(path: str, pattern: str) -> bool:
    pattern = pattern.strip()
    if not pattern:
        return False
    return fnmatch.fnmatch(path, pattern) or fnmatch.fnmatch("/" + path, pattern)


def file_to_markdown(path: Path, title: str) -> str:
    suffix = path.suffix.lower()
    if suffix in {".md", ".markdown", ".mdx"}:
        body = path.read_text(encoding="utf-8", errors="replace").strip()
    elif suffix == ".txt":
        body = path.read_text(encoding="utf-8", errors="replace").strip()
        if not body.startswith("#"):
            body = f"# {title}\n\n{body}"
    elif suffix in PANDOC_EXTENSIONS:
        body = convert_rich_document(path).strip()
        if body and not body.startswith("#"):
            body = f"# {title}\n\n{body}"
    else:
        raise ValueError(f"unsupported extension {suffix}")

    metadata = [
        "> Imported from local folder.",
        f"> - Original filename: `{path.name}`",
        f"> - Original path: `{path}`",
    ]
    try:
        metadata.append(f"> - Modified: `{path.stat().st_mtime_ns}`")
    except OSError:
        pass
    return "\n".join(metadata + ["", body, ""])


def convert_rich_document(path: Path) -> str:
    if shutil.which("pandoc"):
        return convert_with_pandoc(path)
    if path.suffix.lower() == ".docx" and shutil.which("textutil"):
        return convert_docx_with_textutil(path)
    raise RuntimeError(f"{path.suffix} conversion requires pandoc")


def convert_with_pandoc(path: Path) -> str:
    completed = subprocess.run(
        ["pandoc", "-f", pandoc_from_format(path), "-t", "gfm", "--wrap=none", str(path)],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return completed.stdout


def convert_docx_with_textutil(path: Path) -> str:
    completed = subprocess.run(
        ["textutil", "-convert", "txt", "-stdout", str(path)],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return completed.stdout


def pandoc_from_format(path: Path) -> str:
    suffix = path.suffix.lower()
    if suffix in {".html", ".htm"}:
        return "html"
    if suffix == ".docx":
        return "docx"
    raise ValueError(f"no pandoc input format for {suffix}")


def source_path_for_file(path: Path, rel: Path, args: argparse.Namespace) -> str:
    return repo_path(args.source_prefix, rel, path, args.versioned)


def staged_path_for_file(path: Path, rel: Path, args: argparse.Namespace) -> str:
    return repo_path(args.stage_dir, rel, path, args.versioned)


def repo_path(prefix: str, rel: Path, path: Path, versioned: bool) -> str:
    parent_parts = [slugify(part) for part in rel.parts[:-1]]
    stem = slugify(path.stem)
    digest = hashlib.sha256(str(rel).encode("utf-8")).hexdigest()[:10]
    if versioned:
        try:
            version = str(int(path.stat().st_mtime))
        except OSError:
            version = "unknown"
        filename = f"{stem}-{version}-{digest}.md"
    else:
        filename = f"{stem}-{digest}.md"
    return "/".join([prefix.rstrip("/"), *parent_parts, filename])


def title_for_path(path: Path) -> str:
    words = re.split(r"[^A-Za-z0-9]+", path.stem)
    return " ".join(word.capitalize() for word in words if word) or "Untitled"


def slugify(value: str) -> str:
    value = value.strip().lower()
    value = re.sub(r"[^a-z0-9]+", "-", value)
    value = re.sub(r"-+", "-", value).strip("-")
    return value or "untitled"


def run_pi_ingest(brain: Path, prompt: str, args: argparse.Namespace) -> None:
    if args.agent_dry_run:
        print(prompt)
        return
    base_argv = shlex.split(args.pi) + args.pi_arg
    timeout = timeout_seconds(args.pi_timeout)
    if args.pi_json_log:
        argv = base_argv + ["--mode", "json", "-p", prompt]
        stream_pi_json(argv, brain, timeout)
        return

    argv = base_argv + ["-p", prompt]
    try:
        subprocess.run(argv, cwd=brain, text=True, check=True, timeout=timeout)
    except FileNotFoundError as exc:
        raise RuntimeError(f"could not find pi command {argv[0]!r}; pass --pi") from exc
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"pi ingest timed out after {format_timeout(timeout)}: {' '.join(argv[:-1])} <prompt>") from exc
    except subprocess.CalledProcessError as exc:
        raise RuntimeError(f"pi ingest failed with exit code {exc.returncode}: {' '.join(argv)}") from exc


def timeout_seconds(value: float) -> float | None:
    return None if value <= 0 else value


def format_timeout(value: float | None) -> str:
    return "disabled" if value is None else f"{value:g}s"


def stream_pi_json(argv: list[str], cwd: Path, timeout: float | None) -> None:
    print(f"[pi] command: {' '.join(shlex.quote(part) for part in argv[:-1])} <prompt>")
    if timeout is not None:
        print(f"[pi] timeout: {format_timeout(timeout)}")
    try:
        proc = subprocess.Popen(
            argv,
            cwd=cwd,
            text=True,
            stdout=subprocess.PIPE,
            stderr=None,
            bufsize=1,
            start_new_session=True,
        )
    except FileNotFoundError as exc:
        raise RuntimeError(f"could not find pi command {argv[0]!r}; pass --pi") from exc

    timed_out = False

    def on_timeout() -> None:
        nonlocal timed_out
        timed_out = True
        print(f"\n[pi] timeout after {format_timeout(timeout)}; terminating", file=sys.stderr)
        terminate_process_group(proc)

    timer = None
    if timeout is not None:
        timer = threading.Timer(timeout, on_timeout)
        timer.daemon = True
        timer.start()

    assert proc.stdout is not None
    assistant_open = False
    saw_agent_end = False
    try:
        for raw_line in proc.stdout:
            line = raw_line.rstrip("\n")
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                print(f"[pi] {line}")
                continue
            assistant_open = print_pi_event(event, assistant_open)
            if event.get("type") == "agent_end":
                saw_agent_end = True
                break
        if saw_agent_end:
            try:
                code = proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                print("[pi] agent_end received but process did not exit; terminating and treating ingest as complete")
                terminate_process_group(proc)
                proc.wait()
                code = 0
        else:
            code = proc.wait()
    finally:
        if timer is not None:
            timer.cancel()
    if assistant_open:
        print()
    if timed_out:
        raise RuntimeError(f"pi ingest timed out after {format_timeout(timeout)}: {' '.join(argv[:-1])} <prompt>")
    if code != 0:
        raise RuntimeError(f"pi ingest failed with exit code {code}: {' '.join(argv[:-1])} <prompt>")


def terminate_process_group(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        return
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    except Exception:
        proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except Exception:
            proc.kill()


def print_pi_event(event: dict, assistant_open: bool) -> bool:
    event_type = event.get("type")
    if event_type == "session":
        print(f"[pi] session={event.get('id')} cwd={event.get('cwd')}")
    elif event_type == "agent_start":
        print("[pi] agent started")
    elif event_type == "agent_end":
        print("[pi] agent finished")
    elif event_type == "turn_start":
        print("[pi] turn started")
    elif event_type == "turn_end":
        print("[pi] turn finished")
    elif event_type == "tool_execution_start":
        print(f"\n[pi] tool start: {event.get('toolName')} {summarize_json(event.get('args'))}")
    elif event_type == "tool_execution_update":
        partial = event.get("partialResult")
        if partial:
            print(f"[pi] tool update: {event.get('toolName')} {summarize_json(partial, 300)}")
    elif event_type == "tool_execution_end":
        status = "error" if event.get("isError") else "ok"
        print(f"[pi] tool end: {event.get('toolName')} status={status}")
        if event.get("isError"):
            print(f"[pi] tool error result: {summarize_json(event.get('result'), 800)}")
    elif event_type == "message_start":
        message = event.get("message") or {}
        if message.get("role") == "assistant":
            print("[assistant] ", end="", flush=True)
            assistant_open = True
    elif event_type == "message_update":
        update = event.get("assistantMessageEvent") or {}
        if update.get("type") == "text_delta":
            print(update.get("delta", ""), end="", flush=True)
    elif event_type == "message_end":
        message = event.get("message") or {}
        if assistant_open and message.get("role") == "assistant":
            print()
            assistant_open = False
    elif event_type in {"auto_retry_start", "auto_retry_end", "compaction_start", "compaction_end"}:
        print(f"[pi] {event_type}: {summarize_json(event, 500)}")
    return assistant_open


def summarize_json(value, limit: int = 500) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        text = value
    else:
        try:
            text = json.dumps(value, ensure_ascii=False, sort_keys=True)
        except TypeError:
            text = str(value)
    text = re.sub(r"\s+", " ", text).strip()
    if len(text) > limit:
        return text[: limit - 1] + "…"
    return text


def curation_policy_text(policy: str) -> str:
    if policy == "canonical":
        return """- Treat this as current/canonical product documentation unless the file itself says otherwise.
- Prefer durable current concepts, architecture, operations, query language, configuration, and troubleshooting knowledge.
- Do not create one wiki page per docs page by default; update an existing canonical topic or create a small topic page only when it adds reusable knowledge.
- Avoid copying documentation structure, navigation pages, generated reference boilerplate, or low-signal index pages into the wiki."""
    if policy == "historical":
        return """- Treat this as historical context, not current truth.
- Create or update wiki pages only if the document explains durable origin, rationale, decision history, or concepts still needed to understand Tempo.
- Clearly mark synthesized pages with historical/stale/superseded context when applicable.
- Preserve source-only for obsolete operational plans, scratchpads, transient delivery plans, or stale implementation details."""
    if policy == "source-only":
        return """- Preserve the staged document as an immutable Lumbrera source.
- Do not create or update wiki pages.
- Run verification and report that the document was preserved source-only."""
    return """- Use the default lumbrera-ingest policy.
- Prefer small durable source-grounded pages.
- Skip wiki synthesis when the document is low-signal, duplicate, stale, or not useful without broader context."""


def agent_prompt(brain: Path, input_dir: Path, doc: LocalDoc, args: argparse.Namespace) -> str:
    doc_info = {
        "input_dir": str(input_dir),
        "original_path": str(doc.input_path),
        "relative_path": str(doc.relative_path),
        "staged_markdown": doc.staged_rel,
        "lumbrera_source_target": doc.source_rel,
    }
    return f"""/skill:{args.skill_name}

Ingest this staged local document into the Lumbrera brain at `{brain}`.

Curation policy:
{curation_policy_text(args.curation_policy)}

Document metadata:
```json
{json.dumps(doc_info, indent=2, sort_keys=True)}
```

Instructions:
1. Read `{doc.staged_rel}` first. It is a Markdown conversion staged under `.brain/`, not yet a Lumbrera source.
2. Preserve the staged Markdown as an immutable Lumbrera source before creating wiki synthesis:
   `lumbrera write {doc.source_rel} --brain {shlex.quote(str(brain))} --actor {shlex.quote(args.actor)} --reason {shlex.quote(args.reason_prefix + ': ' + str(doc.relative_path))} < {doc.staged_rel}`
3. Immediately after preserving the source, run `lumbrera index --rebuild --brain {shlex.quote(str(brain))}` once so overlap searches see the new source.
4. Then follow the lumbrera-ingest workflow unless the curation policy says source-only: search for overlap, decide whether to update/create/skip wiki pages, choose title/summary/tags, add links and source citations, and write only through `lumbrera write`.
5. Run Lumbrera commands sequentially. Do not launch multiple `lumbrera search`, `lumbrera index`, `lumbrera verify`, or `lumbrera write` commands in parallel; they share the brain lock and concurrent commands can fail.
6. Use `{doc.source_rel}` as file-level provenance with `--source` for any wiki writes created from this document.
7. Run `lumbrera verify --brain {shlex.quote(str(brain))}` when done.
8. Work autonomously. Do not ask follow-up questions or wait for user input; if uncertain, make the conservative choice, skip the risky change, and report the uncertainty.
9. Report source path, wiki pages changed or skipped, tags chosen, uncertainty, and follow-up work, then exit.
"""


if __name__ == "__main__":
    raise SystemExit(main())
