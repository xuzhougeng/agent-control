#!/usr/bin/env python3
import json
import os
import sys
from pathlib import Path

PRESEEDED_SESSION_ID = "11111111-1111-4111-8111-111111111111"


def session_env_root():
    root = Path.home() / ".claude" / "session-env"
    root.mkdir(parents=True, exist_ok=True)
    return root


def session_path(session_id):
    return session_env_root() / session_id


def has_session(session_id):
    if session_id == PRESEEDED_SESSION_ID:
        return True
    return session_path(session_id).exists()


def create_session_marker(session_id):
    if not session_id or session_id == PRESEEDED_SESSION_ID:
        return
    session_path(session_id).write_text("fake-session\n", encoding="utf-8")


def parse_args(argv):
    session_flag = ""
    session_id = ""
    stream_json = False
    i = 0
    while i < len(argv):
        item = argv[i]
        if item in {"-p", "--input-format=stream-json", "--output-format=stream-json"}:
            stream_json = True
        if item in {"--session-id", "--resume"} and i + 1 < len(argv):
            session_flag = item
            session_id = argv[i + 1].strip()
            i += 1
        i += 1
    return session_flag, session_id, stream_json


def extract_user_text(payload):
    try:
        doc = json.loads(payload)
    except json.JSONDecodeError:
        return payload.strip()
    message = doc.get("message") or {}
    content = message.get("content") or []
    parts = []
    for item in content:
        if isinstance(item, dict) and item.get("type") == "text":
            text = str(item.get("text") or "").strip()
            if text:
                parts.append(text)
    return " ".join(parts).strip()


def emit(event):
    sys.stdout.write(json.dumps(event) + "\n")
    sys.stdout.flush()


def fail_stream(text):
    emit({
        "type": "result",
        "is_error": True,
        "errors": [text],
    })
    return 1


def handle_stream(session_flag, session_id):
    payload = sys.stdin.read()
    if session_flag == "--resume" and not has_session(session_id):
        return fail_stream(f"No conversation found with session ID {session_id}.")
    if session_flag == "--session-id" and has_session(session_id):
        return fail_stream(f"Session ID {session_id} is already in use.")
    if session_flag == "--session-id":
        create_session_marker(session_id)

    user_text = extract_user_text(payload) or "(empty)"
    reply = f"[fake-claude:{session_id[:8]}] {user_text}"
    emit({
        "type": "system",
        "subtype": "init",
        "model": "fake-claude",
        "permissionMode": "dontAsk",
        "tools": ["Task", "Read", "Bash"],
    })
    emit({
        "type": "assistant",
        "message": {
            "content": [
                {"type": "text", "text": reply},
            ]
        },
    })
    emit({
        "type": "result",
        "is_error": False,
        "result": reply,
        "duration_ms": 25,
        "num_turns": 1,
        "usage": {"input_tokens": 8, "output_tokens": 8},
    })
    return 0


def handle_pty(session_flag, session_id):
    if session_flag == "--resume" and not has_session(session_id):
        sys.stderr.write(f"Error: No conversation found with session ID {session_id}.\n")
        sys.stderr.flush()
        return 1
    if session_flag == "--session-id" and has_session(session_id):
        sys.stderr.write(f"Error: Session ID {session_id} is already in use.\n")
        sys.stderr.flush()
        return 1

    if session_flag == "--resume":
        sys.stdout.write(f"Resumed session {session_id}\n")
    else:
        sys.stdout.write(f"Started session {session_id}\n")
    sys.stdout.write("Fake Claude PTY ready.\n")
    sys.stdout.flush()

    established = has_session(session_id)
    for raw in sys.stdin:
        line = raw.rstrip("\r\n")
        if not line:
            sys.stdout.write("> \n")
            sys.stdout.flush()
            continue
        if line.lower() in {"exit", "quit"}:
            sys.stdout.write("bye\n")
            sys.stdout.flush()
            return 0
        if not established and session_flag == "--session-id":
            create_session_marker(session_id)
            established = True
        sys.stdout.write(f"echo: {line}\n")
        sys.stdout.flush()
    return 0


def main():
    session_env_root()
    if not session_path(PRESEEDED_SESSION_ID).exists():
        create_session_marker(PRESEEDED_SESSION_ID)
    session_flag, session_id, stream_json = parse_args(sys.argv[1:])
    if stream_json:
        return handle_stream(session_flag, session_id)
    return handle_pty(session_flag, session_id)


if __name__ == "__main__":
    raise SystemExit(main())
