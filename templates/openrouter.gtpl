<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OpenRouter Chat</title>
<script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js"></script>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.css">
<script src="https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/katex.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/katex@0.16.9/dist/contrib/auto-render.min.js"></script>
<style>
 body { font-family: system-ui, sans-serif; background: #f6f8ff; color: #1e2430; margin: 0; padding: 24px; }
 .container { max-width: 1200px; margin: 0 auto; background: #fff; border-radius: 14px; padding: 22px; box-shadow: 0 10px 30px rgba(46,94,169,0.08); }
 h1 { margin-top: 0; color: #1a4c8f; }
 .panel { background: #f7f9fe; border: 1px solid #dfe7f8; border-radius: 10px; padding: 16px; margin-bottom: 18px; }
 .row { display: flex; gap: 12px; flex-wrap: wrap; align-items: center; margin-bottom: 12px; }
 label { font-weight: 600; color: #32507f; }
 select, input, textarea, button { font: inherit; }
 select, input, textarea { border: 1px solid #cfdcf8; padding: 8px 10px; border-radius: 8px; background: white; }
 textarea { width: 62em; max-width: 85vw; min-height: 48px; }
 button { background: linear-gradient(180deg, #2d6cdf 0%, #2257b5 100%); color: white; border: none; border-radius: 8px; padding: 8px 14px; cursor: pointer; }
 button.secondary { background: linear-gradient(180deg, #8293b5 0%, #677ca0 100%); }
 .history { border-top: 1px solid #e4ecff; margin-top: 16px; padding-top: 16px; }
 .meta { font-size: 12px; color: #57709a; margin-bottom: 8px; }
 .markdown { display: none; border: 1px solid #dfe8f8; border-radius: 8px; background: #fbfcff; width: 62em; max-width: 85vw; padding: 14px; box-sizing: border-box; overflow-wrap: anywhere; }
 .markdown pre { overflow-x: auto; background: #eef4ff; padding: 10px; border-radius: 6px; }
 .markdown code { background: #edf4ff; padding: 2px 6px; border-radius: 4px; }
</style>
</head>
<body>
<div class="container">
<h1>OpenRouter Chat</h1>
<form method="POST" action="/openrouter" enctype="multipart/form-data">
<div class="panel">
<div class="row">
<label>System</label>
<select name="system">
{{ range .SystemKeys }}
<option value="{{ . }}" {{ if eq $.SelectedSystem . }}selected{{ end }}>{{ . }}</option>
{{ end }}
</select>
<label>Model</label>
<select name="model">
{{ range .ModelList }}
<option value="{{ .Name }}" {{ if eq $.SelectedModel .Name }}selected{{ end }}>{{ .Name }} ({{ .Vendor }})</option>
{{ end }}
</select>
<label>Max Tokens</label>
<input type="number" name="maxtokens" value="2000" style="width: 7em;">
<label>Temperature</label>
<input type="number" name="temperature" value="0.2" min="0" max="2" step="0.1" style="width: 6em;">
</div>
<div class="row">
<label>質問</label>
<textarea name="question" rows="3">{{ .Question }}</textarea>
</div>
<div class="row" style="margin-top: -6px;">
<span style="font-size: 12px; color: #57709a;">履歴一覧でチェックしたQAだけを、次の質問の文脈として送信します。</span>
</div>
<div class="row">
<button type="submit">送信</button>
<button type="submit" name="action" value="clearchecks" class="secondary">チェックをすべて解除</button>
</div>
</div>
<div class="panel">
<div class="row">
<label>検索</label>
<input type="text" name="target" value="{{ .Target }}" style="width: 30em;">
<button type="submit" class="secondary">検索</button>
<label>Start ID</label>
<input type="number" name="startid" value="{{ .StartID }}" style="width: 8em;">
<button type="submit" class="secondary">移動</button>
</div>
<div class="row">
{{ if .HasPrevious }}<button type="submit" name="action" value="previous" class="secondary">前へ</button>{{ end }}
{{ if .HasNext }}<button type="submit" name="action" value="next" class="secondary">次へ</button>{{ end }}
</div>
</div>
{{ if .Error }}<div style="color: #a22828;">{{ .Error }}</div>{{ end }}
{{ if .Info }}<div style="color: #1d5a2a;">{{ .Info }}</div>{{ end }}
<div class="history">
{{ range .Records }}
<div style="margin-bottom: 18px; padding-bottom: 14px; border-bottom: 1px solid #eef2ff;">
<div class="meta">
<label style="font-weight: 500; color: #57709a;"><input type="checkbox" name="historyid" value="{{ .Id }}" {{ if index $.SelectedIDs .Id }}checked{{ end }}> context</label>
 / id={{ .Id }} / model={{ .Modelname }} / {{ TimeToStringY .Timestamp }} / {{ printf "%5.1f" (msToSeconds .Responsetime) }}s / prompt={{ .Itokens }} / completion={{ .Otokens }}</div>
<div class="row">
<textarea readonly rows="3">{{ .Question }}</textarea>
<button type="button" onclick="toggleMarkdown(this.previousElementSibling, this.nextElementSibling)">TXT</button>
<div class="markdown"></div>
</div>
<div class="row">
<textarea readonly rows="6">{{ .Answer }}</textarea>
<button type="button" onclick="toggleMarkdown(this.previousElementSibling, this.nextElementSibling)">TXT</button>
<div class="markdown"></div>
</div>
</div>
{{ end }}
</div>
</form>
</div>
<script>
function toggleMarkdown(textArea, mdBox) {
	const button = textArea.nextElementSibling;
	if (textArea.style.display === 'none') {
		textArea.style.display = 'block';
		mdBox.style.display = 'none';
		button.textContent = 'TXT';
		return;
	}
	let raw = textArea.value;
	const mathBlocks = [];
	raw = raw.replace(/(\\\[[\s\S]*?\\\]|\\\(.*?\\\))|(\$\$[\s\S]*?\$\$|\$.*?\$)/g, (match) => {
		const placeholder = 'MATH_' + mathBlocks.length;
		mathBlocks.push(match);
		return placeholder;
	});
	let html = marked.parse(raw);
	mathBlocks.forEach((math, idx) => {
		html = html.replace('MATH_' + idx, math);
	});
	mdBox.innerHTML = html;
	renderMathInElement(mdBox, {
		delimiters: [
			{ left: '$$', right: '$$', display: true },
			{ left: '$', right: '$', display: false },
			{ left: '\\(', right: '\\)', display: false },
			{ left: '\\[', right: '\\]', display: true }
		],
		throwOnError: false
	});
	textArea.style.display = 'none';
	mdBox.style.display = 'block';
	button.textContent = 'MD';
}
</script>
