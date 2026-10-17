"""Model-free audit probes; virtual Go test files never modify application files."""

import json
import os
import subprocess
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
PACKAGES = ("epub", "analysis", "webapp", "cardexport", "enrichment")
scratch = Path.home() / ".cache/scratch/poetry-audit-repro"
scratch.mkdir(parents=True, exist_ok=True)
try:
    with tempfile.TemporaryDirectory(dir=scratch) as directory:
        work = Path(directory)
        overlay = {
            str(ROOT / "internal" / name / "poetry_audit_probe_test.go"): str(
                HERE / f"{name}.go.txt"
            )
            for name in PACKAGES
        }
        config = work / "overlay.json"
        config.write_text(json.dumps({"Replace": overlay}))
        env = dict(os.environ, TMPDIR=directory)
        env.setdefault("GOTMPDIR", str(Path.home() / ".cache/gotmp"))
        Path(env["GOTMPDIR"]).mkdir(parents=True, exist_ok=True)
        subprocess.run(
            [
                "go",
                "test",
                "-count=1",
                f"-overlay={config}",
                "-run=^TestPoetryAudit",
                "-v",
            ]
            + [f"./internal/{name}" for name in PACKAGES],
            cwd=ROOT,
            env=env,
            check=True,
        )
finally:
    # Only this runner's empty parent directory; never remove another run's work.
    if scratch.exists() and not any(scratch.iterdir()):
        scratch.rmdir()
