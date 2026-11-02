#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
从水情基础库导出「水库基础信息 + 水位-库容曲线」为 reservoir_data.json

数据源（10.33.120.181:8527，Navicat 连接名 LH_Water_JDBC）：
  sjy_project_basic.att_res_lm_base      大中型水库基础信息
  sjy_project_basic.att_res_s_base       小型水库基础信息
  sjy_project_basic.fhgc_res_lm_rwacdr   大中型水库水位、面积、库容、泄量关系
  sjy_project_basic.fhgc_res_s_rwacdr    小型水库水位、面积、库容、泄量关系

单位：水位 m，库容 万m³

用法：
  set DB_HOST=10.33.120.181
  set DB_PORT=8527
  set DB_USER=lh_water_basic
  set DB_PASS=******
  python export_from_db.py
"""
import json
import os
import sys
from datetime import datetime

import pymysql

HOST = os.environ.get("DB_HOST", "10.33.120.181")
PORT = int(os.environ.get("DB_PORT", "8527"))
USER = os.environ.get("DB_USER", "lh_water_basic")
PASS = os.environ.get("DB_PASS", "")
DB = os.environ.get("DB_NAME", "sjy_project_basic")

SCALE_MAP = {
    "1": "大(一)型",
    "2": "大(二)型",
    "3": "中型",
    "4": "小(一)型",
    "5": "小(二)型",
}

LM_SQL = """
SELECT b.id, b.RES_NAME, b.ENG_SCAL, b.ENG_GRAD, b.RES_LOC, b.ADCD, b.RCDS_AREA, b.CON_PRO_CODE,
       b.DEAD_LEV, b.DEAD_CAP, b.NORM_WAT_LEV, b.NORM_POOL_STAG_CAP,
       b.CHFLLV, b.TOT_CAP, b.FLCO_CAP, b.DES_FL_STAG
FROM att_res_lm_base b
WHERE b.is_delete = '0'
"""

S_SQL = """
SELECT b.id, b.RES_NAME, b.ENG_SCAL, b.ENG_GRAD, b.RES_LOC, b.ADCD, b.RCDS_AREA, b.CON_PRO_CODE,
       b.DEAD_LEV, b.DEAD_LEV_CAP, b.NORM_WAT_LEV, b.NORM_POOL_STAG_CAP,
       b.CHFLLV, b.TOT_CAP, b.BEN_RES_CAP, b.DES_FL_STAG
FROM att_res_s_base b
WHERE b.is_delete = '0'
"""


def f(v):
    return None if v is None else float(v)


def clean_curve(rows):
    """rows: [(WTLV, RSCP)] -> 按水位升序、库容单调不减的折线点"""
    pts = {}
    for z, v in rows:
        if z is None or v is None:
            continue
        z = round(float(z), 3)
        v = round(float(v), 2)
        if v < 0:
            continue
        # 同一水位多条记录取均值
        pts.setdefault(z, []).append(v)
    seq = sorted((z, sum(vs) / len(vs)) for z, vs in pts.items())
    out = []
    run = -1.0
    for z, v in seq:
        if v < run:          # 库容倒退 -> 用前值压平，保证单调
            v = run
        run = v
        out.append([z, round(v, 2)])
    # 去掉库容完全相同的中间冗余点
    dedup = []
    for p in out:
        if len(dedup) >= 2 and dedup[-1][1] == p[1] and dedup[-2][1] == p[1]:
            dedup[-1] = p
            continue
        dedup.append(p)
    return dedup


def main():
    conn = pymysql.connect(host=HOST, port=PORT, user=USER, password=PASS,
                           database=DB, charset="utf8mb4", connect_timeout=15)
    cur = conn.cursor()

    reservoirs = []
    stats = {"lm": 0, "s": 0, "no_curve": 0, "fixed_mono": 0}

    for kind, sql, curve_tbl in (("大中型", LM_SQL, "fhgc_res_lm_rwacdr"),
                                 ("小型", S_SQL, "fhgc_res_s_rwacdr")):
        cur.execute(sql)
        bases = cur.fetchall()
        cur.execute("SELECT RES_ID, WTLV, RSCP FROM %s ORDER BY RES_ID, WTLV" % curve_tbl)
        curves = {}
        for rid, z, v in cur.fetchall():
            curves.setdefault(rid, []).append((z, v))

        for row in bases:
            (rid, name, scal, grad, loc, adcd, area, code,
             dead_z, dead_v, norm_z, norm_v, chk_z, tot_v, flood_v, des_z) = row
            pts = clean_curve(curves.get(rid, []))
            if not pts:
                stats["no_curve"] += 1
            reservoirs.append({
                "id": rid,
                "code": (code or "").strip(),
                "name": (name or "").strip(),
                "kind": kind,
                "scale": SCALE_MAP.get(str(scal), str(scal) if scal else ""),
                "grade": (grad or "").strip(),
                "loc": (loc or "").strip(),
                "adcd": (adcd or "").strip(),
                "area": f(area),
                "deadZ": f(dead_z),
                "deadV": f(dead_v),
                "normZ": f(norm_z),
                "normV": f(norm_v),
                "chkZ": f(chk_z),
                "totalV": f(tot_v),
                "floodV": f(flood_v),
                "desZ": f(des_z),
                "pts": pts,
            })
            stats["lm" if kind == "大中型" else "s"] += 1

    reservoirs.sort(key=lambda r: (r["kind"] != "大中型", r["scale"], r["name"]))
    payload = {
        "meta": {
            "source": "%s:%d / %s" % (HOST, PORT, DB),
            "tables": ["att_res_lm_base", "att_res_s_base",
                       "fhgc_res_lm_rwacdr", "fhgc_res_s_rwacdr"],
            "unitZ": "m",
            "unitV": "万m³",
            "exportedAt": datetime.now().strftime("%Y-%m-%d %H:%M"),
            "count": len(reservoirs),
            "points": sum(len(r["pts"]) for r in reservoirs),
        },
        "reservoirs": reservoirs,
    }

    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "reservoir_data.json")
    with open(out, "w", encoding="utf-8") as fp:
        json.dump(payload, fp, ensure_ascii=False, separators=(",", ":"))
    print("written:", out, os.path.getsize(out) // 1024, "KB")
    print("reservoirs:", len(reservoirs), "points:", payload["meta"]["points"], stats)
    conn.close()


if __name__ == "__main__":
    sys.exit(main())
