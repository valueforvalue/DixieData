import json
import re
import sys
from pathlib import Path


PATTERNS = {
    "database_locked": re.compile(r"database is locked", re.IGNORECASE),
    "panic": re.compile(r"\bpanic:\b", re.IGNORECASE),
    "goroutine_leak": re.compile(r"goroutine", re.IGNORECASE),
    "recovered_panic": re.compile(r"recovered", re.IGNORECASE),
    "white_screen": re.compile(r"white[- ]screen|timeout waiting for response|hang suspected", re.IGNORECASE),
}


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: analyze_stress_log.py <stress_test.log>", file=sys.stderr)
        return 2

    log_path = Path(sys.argv[1])
    if not log_path.exists():
        print(f"log not found: {log_path}", file=sys.stderr)
        return 2

    text = log_path.read_text(encoding="utf-8", errors="replace")
    summary = {name: len(pattern.findall(text)) for name, pattern in PATTERNS.items()}
    print(json.dumps(summary, indent=2))

    suspicious = [name for name, count in summary.items() if count > 0]
    if suspicious:
        print("suspicious patterns detected: " + ", ".join(suspicious), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
