---
title: "TermCast MCP: From Markdown to GIF"
description: "Record a lab to JSONL, bind Markdown, export a GitHub Dark cast, and render a GIF."
type: tutorial
layout: codelab
---

# TermCast MCP: Markdown to animated GIF

This demonstration uses the `./mcp-term` executable from the repository root, plus `python3` and `agg` installed on `PATH`. Run from the repository root. The demo uses the `termcast-demo/` directory; remove or rename an existing directory before repeating the demo. **Import executes commands**, so review the lab first.

{{< step label="Create a sample Markdown lab" >}}

### 1. Create a sample lab without nested Markdown fences

The generator builds the Markdown code-fence delimiters at runtime, so the outer lab importer does not mistake the sample lab's commands for commands in this demonstration.

```bash {terminal="true" typing="35ms"}
python3 - <<'PY'
from pathlib import Path
p = Path('termcast-demo')
p.mkdir(exist_ok=True)
fence = chr(96) * 3
lab = ('# Hello from TermCast MCP\n\n'
       '{{< step label="Run a greeting" >}}\n\n'
       '### Print a greeting\n\n'
       + fence + 'bash {terminal="true"}\n'
       + "printf 'Hello from TermCast MCP!\\n'\n"
       + fence + '\n\n'
       '### Show the working directory\n\n'
       + fence + 'bash {terminal="true"}\n'
       + 'pwd\n'
       + fence + '\n\n'
       '{{< /step >}}\n')
(p / 'sample-lab.md').write_text(lab)
print('Created termcast-demo/sample-lab.md')
PY
```

{{< /step >}}

{{< step label="Record commands to JSONL" >}}

### 2. Execute the sample lab and record its output

```bash {terminal="true" typing="35ms"}
./mcp-term lab-import --input termcast-demo/sample-lab.md --log termcast-demo/sample-lab.jsonl
```

### 3. Inspect the JSONL recording

```bash {terminal="true" typing="35ms"}
wc -l termcast-demo/sample-lab.jsonl
```

```info
The JSONL recording preserves execution evidence. Binding and exporting use that evidence without rerunning the commands.
```

{{< /step >}}

{{< step label="Bind Markdown to the recording" >}}

### 4. Bind the sample lab without re-execution

```bash {terminal="true" typing="35ms"}
./mcp-term lab-import --input termcast-demo/sample-lab.md --log termcast-demo/sample-lab.jsonl --output termcast-demo/sample-lab-bound.md --bind-only
```

### 5. Inspect the command references

```bash {terminal="true" typing="35ms"}
grep 'command-id=' termcast-demo/sample-lab-bound.md
```

```hint
The bound Markdown points to the JSONL record; editing explanatory text does not require rerunning the lab.
```

{{< /step >}}

{{< step label="Export a GitHub Dark cast" >}}

### 6. Export the bound walkthrough to asciinema v2

```bash {terminal="true" typing="35ms"}
./mcp-term export-asciinema --lab termcast-demo/sample-lab-bound.md --log termcast-demo/sample-lab.jsonl --output termcast-demo/sample-lab.cast --presentation tutorial --theme github-dark --source-reveal smooth
```

### 7. Verify the cast exists

```bash {terminal="true" typing="35ms"}
ls -lh termcast-demo/sample-lab.cast
```

```info
The github-dark theme is applied during cast export. Exporting does not execute commands.
```

{{< /step >}}

{{< step label="Convert the cast to GIF" >}}

### 8. Render the animated GIF using agg

```bash {terminal="true" typing="35ms"}
agg --font-size 64 termcast-demo/sample-lab.cast termcast-demo/sample-lab.gif
```

### 9. Verify the GIF exists

```bash {terminal="true" typing="35ms"}
ls -lh termcast-demo/sample-lab.gif
```

```hint
agg renders the ANSI-styled cast; it does not need a separate github-dark option.
```

{{< /step >}}

## Capture this entire workflow

From the repository root, run these commands outside the walkthrough to create a second cast and GIF that demonstrate the complete pipeline:

```sh
./mcp-term lab-import \
  --input mcp-term-self-demo.md \
  --log mcp-term-self-demo.jsonl \
  --output mcp-term-self-demo-bound.md

./mcp-term export-asciinema \
  --lab mcp-term-self-demo-bound.md \
  --log mcp-term-self-demo.jsonl \
  --output mcp-term-self-demo.cast \
  --presentation tutorial \
  --theme github-dark \
  --source-reveal smooth

agg --font-size 64 mcp-term-self-demo.cast screenshots/mcp-term-self-demo.gif
```
