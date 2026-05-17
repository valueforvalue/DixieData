import argparse
import json
import shutil
import sqlite3
import subprocess
from datetime import datetime, timezone
from pathlib import Path


def run(command, cwd: Path, stdout_path: Path | None = None) -> None:
    print("==>", " ".join(command))
    if stdout_path is None:
        subprocess.run(command, cwd=cwd, check=True)
        return
    with stdout_path.open("w", encoding="utf-8") as handle:
        subprocess.run(command, cwd=cwd, check=True, stdout=handle, stderr=subprocess.STDOUT)


def load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def bulk_fill_archive(db_path: Path, target_soldiers: int, seeded_soldiers: int) -> int:
    remaining = max(target_soldiers - seeded_soldiers, 0)
    if remaining == 0:
        return 0

    connection = sqlite3.connect(db_path)
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    inserted = 0
    try:
        cursor = connection.cursor()
        batch: list[tuple[str, str, str, int, str, str, str, str, str, str, str]] = []
        for offset in range(remaining):
            sequence = seeded_soldiers + offset + 1
            batch.append(
                (
                    f"BULK-{sequence:05d}",
                    f"bulk-sync-{sequence:05d}",
                    "soldier",
                    1,
                    "Bulk",
                    f"Soldier-{sequence:05d}",
                    "Stress Regiment",
                    "Virginia",
                    "None",
                    now,
                    now,
                )
            )
            if len(batch) == 1000:
                cursor.executemany(
                    """
                    INSERT INTO soldiers (
                        display_id, sync_id, entry_type, is_generated,
                        first_name, last_name, unit, pension_state,
                        confederate_home_status, created_at, updated_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """,
                    batch,
                )
                connection.commit()
                inserted += len(batch)
                batch.clear()
        if batch:
            cursor.executemany(
                """
                INSERT INTO soldiers (
                    display_id, sync_id, entry_type, is_generated,
                    first_name, last_name, unit, pension_state,
                    confederate_home_status, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                batch,
            )
            connection.commit()
            inserted += len(batch)
    finally:
        connection.close()
    return inserted


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", default=".")
    parser.add_argument("--report-dir", default=r".\tests\goldmaster\artifacts\stress")
    parser.add_argument("--data-dir", default=r".\tests\goldmaster\artifacts\stress\data")
    parser.add_argument("--soldiers", type=int, default=7500)
    parser.add_argument("--seed-soldiers", type=int, default=1500)
    parser.add_argument("--seed", type=int, default=1865)
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    report_dir = Path(args.report_dir).resolve()
    data_dir = Path(args.data_dir).resolve()

    if report_dir.exists():
        shutil.rmtree(report_dir)
    report_dir.mkdir(parents=True, exist_ok=True)
    data_dir.mkdir(parents=True, exist_ok=True)

    seeded_soldiers = min(args.soldiers, max(args.seed_soldiers, 1))
    seed_log = report_dir / "seed.log"
    run(
        [
            "go",
            "run",
            r".\cmd\seed-data",
            "-data-dir",
            str(data_dir),
            "-reset",
            "-soldiers",
            str(seeded_soldiers),
            "-seed",
            str(args.seed),
        ],
        repo_root,
        seed_log,
    )

    db_path = data_dir / "dixiedata.db"
    bulk_inserted = bulk_fill_archive(db_path, args.soldiers, seeded_soldiers)

    benchmark_log = report_dir / "benchmark.log"
    run(
        [
            "go",
            "run",
            r".\cmd\gold-master",
            "-mode",
            "benchmark",
            "-data-dir",
            str(data_dir),
            "-report-dir",
            str(report_dir),
        ],
        repo_root,
        benchmark_log,
    )

    stress_artifact_root = report_dir / "stress-artifacts"
    run(
        [
            "powershell",
            "-NoProfile",
            "-ExecutionPolicy",
            "Bypass",
            "-File",
            r".\run-stress-tests.ps1",
            "-ArtifactRoot",
            str(stress_artifact_root),
        ],
        repo_root,
    )

    benchmark = load_json(report_dir / "report.json")
    summary = {
        "seed_log": str(seed_log),
        "seeded_soldiers": seeded_soldiers,
        "bulk_inserted_soldiers": bulk_inserted,
        "benchmark": benchmark.get("metrics", {}),
        "stress_artifacts": str(stress_artifact_root),
    }
    (report_dir / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    print(json.dumps(summary, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
