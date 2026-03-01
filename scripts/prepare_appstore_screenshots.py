#!/usr/bin/env python3
"""
将截屏 PNG 转换为 App Store Connect 所需尺寸：
  - 1242×2688 (6.5" 竖屏)
  - 1284×2778 (6.7" 竖屏)
策略：等比缩放后居中，边缘用截屏角落颜色填充。
"""

import argparse
from pathlib import Path
from PIL import Image

TARGETS = {
    "6.5inch": (1242, 2688),
    "6.7inch": (1284, 2778),
}


def parse_args():
    p = argparse.ArgumentParser(description="将目录下 PNG 转为 App Store 截屏尺寸")
    p.add_argument(
        "-i", "--input-dir",
        type=Path,
        default=Path.home() / "Desktop",
        help="输入目录，处理其下所有 .png 文件（默认: 桌面）",
    )
    p.add_argument(
        "-o", "--output-dir",
        type=Path,
        default=Path.home() / "Desktop" / "AppStore_Screenshots",
        help="输出目录（默认: 桌面/AppStore_Screenshots）",
    )
    return p.parse_args()


def sample_bg_color(img: Image.Image) -> tuple:
    """从四个角取样并返回平均背景色。"""
    w, h = img.size
    corners = [
        img.getpixel((5, 5)),
        img.getpixel((w - 5, 5)),
        img.getpixel((5, h - 5)),
        img.getpixel((w - 5, h - 5)),
    ]
    # 只取 RGB
    corners = [c[:3] if len(c) == 4 else c for c in corners]
    r = sum(c[0] for c in corners) // 4
    g = sum(c[1] for c in corners) // 4
    b = sum(c[2] for c in corners) // 4
    return (r, g, b)


def fit_and_pad(src: Image.Image, target_w: int, target_h: int) -> Image.Image:
    """等比缩放 src 以填满目标尺寸（cover），然后居中裁剪；
    若比例极近则缩放后填充背景色。"""
    src_ratio = src.width / src.height
    tgt_ratio = target_w / target_h

    bg_color = sample_bg_color(src)
    canvas = Image.new("RGB", (target_w, target_h), bg_color)

    # 缩放：使图像完全适合目标（fit，不裁剪）
    scale = min(target_w / src.width, target_h / src.height)
    new_w = round(src.width * scale)
    new_h = round(src.height * scale)
    resized = src.resize((new_w, new_h), Image.LANCZOS)

    # 居中贴到画布
    offset_x = (target_w - new_w) // 2
    offset_y = (target_h - new_h) // 2
    canvas.paste(resized, (offset_x, offset_y))
    return canvas


def main():
    args = parse_args()
    input_dir = args.input_dir.resolve()
    output_dir = args.output_dir.resolve()

    if not input_dir.is_dir():
        print(f"错误：输入目录不存在: {input_dir}")
        return

    sources = sorted(input_dir.glob("*.png"))
    if not sources:
        print(f"未在 {input_dir} 下找到 .png 文件")
        return

    output_dir.mkdir(parents=True, exist_ok=True)
    print(f"找到 {len(sources)} 张 PNG，输出至 {output_dir}\n")

    for src_path in sources:
        img = Image.open(src_path).convert("RGB")
        print(f"处理: {src_path.name}  ({img.width}×{img.height})")

        for label, (tw, th) in TARGETS.items():
            out_img = fit_and_pad(img, tw, th)
            stem = src_path.stem
            out_name = f"{stem}_{label}_{tw}x{th}.png"
            out_path = output_dir / out_name
            out_img.save(out_path, "PNG", optimize=True)
            print(f"  -> {label}: {out_path.name}")

        print()

    print(f"完成！共生成 {len(sources) * len(TARGETS)} 张截屏。")
    print(f"输出目录: {output_dir}")


if __name__ == "__main__":
    main()
