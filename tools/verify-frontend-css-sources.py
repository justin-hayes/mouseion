#!/usr/bin/env python3
"""Prove the Tailwind source boundary through isolated compiler output."""

from __future__ import annotations

import hashlib
import shutil
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
WEBAPP = ROOT / "internal" / "webapp"
TOOL_CACHE = ROOT / ".tmp" / "frontend-css"


def require(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


def selector(candidate: str) -> str:
    return "." + candidate.replace("[", r"\[").replace("]", r"\]")


def main() -> None:
    # Use the same checksum-verified standalone compiler as the production build.
    compiler = Path(
        subprocess.check_output(
            [str(ROOT / "tools" / "build-frontend-css.sh"), "--print-tailwind-path"],
            text=True,
        ).strip()
    )
    require(compiler.is_file(), f"Tailwind compiler not found: {compiler}")

    with tempfile.TemporaryDirectory(prefix="frontend-css-sources-", dir=ROOT / ".tmp") as temp:
        fixture_root = Path(temp)
        fixture_webapp = fixture_root / "internal" / "webapp"
        fixture_styles = fixture_webapp / "styles"
        fixture_styles.mkdir(parents=True)
        for path in WEBAPP.glob("*.templ"):
            shutil.copy2(path, fixture_webapp / path.name)
        for name in ("components.go",):
            shutil.copy2(WEBAPP / name, fixture_webapp / name)
        (fixture_webapp / "static").mkdir()
        shutil.copy2(WEBAPP / "static" / "my-books.js", fixture_webapp / "static" / "my-books.js")
        shutil.copytree(WEBAPP / "styles", fixture_styles, dirs_exist_ok=True)

        entry = fixture_styles / "app.css"
        css = entry.read_text(encoding="utf-8")
        css = css.replace(
            '"../../../.tmp/frontend-css/daisyui-5.0.50.mjs"',
            f'"{(TOOL_CACHE / "daisyui-5.0.50.mjs").as_posix()}"',
        )
        entry.write_text(css, encoding="utf-8")

        def compile_to(output: Path) -> None:
            subprocess.run(
                [str(compiler), "--input", str(entry), "--output", str(output)],
                cwd=fixture_styles,
                check=True,
                stdout=subprocess.DEVNULL,
            )

        # The pristine copy must match the committed artifact. A temporary
        # authored utility below must make that snapshot stale, just as the
        # freshness check detects a changed source; rebuilding then produces
        # the same output twice.
        committed_css = (WEBAPP / "static" / "app.css").read_bytes()
        pristine_output = fixture_root / "pristine.css"
        compile_to(pristine_output)
        require(
            pristine_output.read_bytes() == committed_css,
            "committed stylesheet is stale before the source-change probe",
        )

        # The temporary-only markers prove that every declared source group is
        # scanned, while generated/tests/vendor/temporary inputs stay excluded.
        included = {
            "views.templ": "w-[731px]",
            "new-authored-view.templ": "w-[732px]",
            "lemma_review.templ": "w-[733px]",
            "components.go": "w-[734px]",
            "static/my-books.js": "w-[735px]",
        }
        for relative, candidate in included.items():
            path = fixture_webapp / relative
            if not path.exists():
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("", encoding="utf-8")
            with path.open("a", encoding="utf-8") as source:
                source.write(f"\n/* class: {candidate} */\n")

        excluded = {
            "views_templ.go": "w-[741px]",
            "tests/source_test.go": "w-[742px]",
            "static/vendor/third-party.js": "w-[743px]",
            "temporary.html": "w-[744px]",
        }
        for relative, candidate in excluded.items():
            path = fixture_webapp / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(f'<div class="{candidate}"></div>\n', encoding="utf-8")

        output_a = fixture_root / "first.css"
        output_b = fixture_root / "second.css"
        compile_to(output_a)
        compile_to(output_b)
        first = output_a.read_text(encoding="utf-8")
        second = output_b.read_text(encoding="utf-8")

        for relative, candidate in included.items():
            require(selector(candidate) in first, f"declared source not emitted ({relative}): {candidate}")
        for relative, candidate in excluded.items():
            require(selector(candidate) not in first, f"undeclared source leaked into CSS ({relative}): {candidate}")
        require(first != committed_css.decode("utf-8"), "authored source change did not make the committed stylesheet stale")
        require(
            hashlib.sha256(output_a.read_bytes()).digest()
            == hashlib.sha256(output_b.read_bytes()).digest(),
            "identical authored inputs produced different CSS output",
        )

    print("Frontend CSS source boundary is explicit, bounded, and reproducible.")


if __name__ == "__main__":
    main()
