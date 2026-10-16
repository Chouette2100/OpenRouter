<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>OpenRouter Login</title>
<style>
 body { font-family: system-ui, sans-serif; background: #f6f8ff; color: #1e2430; margin: 0; padding: 24px; }
 .container { max-width: 520px; margin: 48px auto; background: #fff; border-radius: 14px; padding: 24px; box-shadow: 0 10px 30px rgba(46,94,169,0.08); }
 h1 { margin-top: 0; color: #1a4c8f; }
 .row { display: flex; flex-direction: column; gap: 6px; margin-bottom: 14px; }
 label { font-weight: 600; color: #32507f; }
 input, button { font: inherit; }
 input { border: 1px solid #cfdcf8; padding: 10px; border-radius: 8px; background: white; }
 button { background: linear-gradient(180deg, #2d6cdf 0%, #2257b5 100%); color: white; border: none; border-radius: 8px; padding: 10px 14px; cursor: pointer; }
</style>
</head>
<body>
<div class="container">
<h1>OpenRouter Login</h1>
<form action="/login" method="post">
<div class="row">
<label for="email">Email</label>
<input type="email" id="email" name="email" required>
</div>
<div class="row">
<label for="password">Password</label>
<input type="password" id="password" name="password" required>
</div>
<button type="submit">Login</button>
</form>
</div>
</body>
</html>
