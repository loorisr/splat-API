import argparse
from pathlib import Path

import matplotlib.pyplot as plt
from matplotlib.colors import LinearSegmentedColormap
import numpy as np

API_COLORMAPS = [
    "heat",
    "jet",
    "turbo",
    "viridis",
    "magma",
    "plasma",
    "inferno",
    "hot",
    "parula",
    "gray",
    "hsv",
    "cubehelix",
    "cividis",
    "github",
]

MATPLOTLIB_ALIASES = {
    # signalserver name -> matplotlib equivalent
    "heat": "gist_heat",
}


def _resolve_colormap(name: str):
    if name == "parula":
        return LinearSegmentedColormap.from_list(
            "parula",
            ["#352a87", "#0f5cdd", "#00a6ca", "#53c567", "#f9fb0e"],
            N=256,
        )

    if name == "github":
        # GitHub contribution-style scale
        return LinearSegmentedColormap.from_list(
            "github",
            ["#ebedf0", "#c6e48b", "#7bc96f", "#239a3b", "#196127"],
            N=256,
        )

    mpl_name = MATPLOTLIB_ALIASES.get(name, name)
    return plt.get_cmap(mpl_name)


def export_colormap(colormap: str, dimensions: tuple[int, int], filename: Path):
    try:
        width, height = dimensions
        fig, ax = plt.subplots(figsize=(width / 100, height / 100), dpi=100)

        gradient = np.linspace(0, 1, 256).reshape(1, -1)
        ax.imshow(gradient, aspect="auto", cmap=_resolve_colormap(colormap))

        ax.set_axis_off()
        filename.parent.mkdir(parents=True, exist_ok=True)

        plt.subplots_adjust(left=0, right=1, top=1, bottom=0)
        plt.savefig(filename, bbox_inches="tight", pad_inches=0)
        plt.close(fig)
        print(f"Colormap '{colormap}' exported to {filename}.")

    except ValueError as e:
        print(f"Error: '{colormap}' is not a valid colormap. Details: {e}")
    except Exception as e:
        print(f"An error occurred: {e}")


def export_all_colormaps(output_dir: Path, dimensions: tuple[int, int]):
    for colormap in API_COLORMAPS:
        export_colormap(colormap, dimensions, output_dir / f"{colormap}.png")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Export colormap previews to PNG")
    parser.add_argument("colormap", nargs="?", type=str, help="Colormap name (e.g., 'viridis')")
    parser.add_argument("width", nargs="?", type=int, default=256, help="Output width in pixels")
    parser.add_argument("height", nargs="?", type=int, default=30, help="Output height in pixels")
    parser.add_argument("filename", nargs="?", type=str, help="Output filename")
    parser.add_argument(
        "--all",
        action="store_true",
        help="Generate all API colormaps into --output-dir",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default="public/colormaps",
        help="Output directory used with --all or when filename is omitted",
    )

    args = parser.parse_args()

    if args.all:
        export_all_colormaps(Path(args.output_dir), (args.width, args.height))
    else:
        if not args.colormap:
            raise SystemExit("Error: 'colormap' is required when --all is not used.")
        filename = Path(args.filename) if args.filename else Path(args.output_dir) / f"{args.colormap}.png"
        export_colormap(args.colormap, (args.width, args.height), filename)
