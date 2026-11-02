#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""把 reservoir_data.json 注入 template.html，生成可直接双击打开的单文件工具 index.html

同时把成品同步到前端静态目录 frontend/public/reservoir-curve-tool.html，
供 WeKnora「工具箱 → 水库库容」面板用 iframe 加载。
"""
import os
import shutil

here = os.path.dirname(os.path.abspath(__file__))
tpl = open(os.path.join(here, "template.html"), encoding="utf-8").read()
data = open(os.path.join(here, "reservoir_data.json"), encoding="utf-8").read()

if "/*__DATA__*/" not in tpl:
    raise SystemExit("template.html 缺少 /*__DATA__*/ 占位符")

out = os.path.join(here, "index.html")
with open(out, "w", encoding="utf-8") as fp:
    fp.write(tpl.replace("/*__DATA__*/", data))
print("built:", out, os.path.getsize(out) // 1024, "KB")

public_dir = os.path.normpath(os.path.join(here, "..", "frontend", "public"))
if os.path.isdir(public_dir):
    target = os.path.join(public_dir, "reservoir-curve-tool.html")
    shutil.copyfile(out, target)
    print("synced:", target)
else:
    print("skip sync: frontend/public not found")
