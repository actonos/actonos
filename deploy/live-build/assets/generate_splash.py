import os
import struct
import zlib
import math

def write_png(output_path, width, height, pixels):
    """
    Writes raw RGB pixel array into standard uncompressed/deflated PNG.
    pixels: bytes object of length (width * 3 + 1) * height (each row prefixed with filter byte 0)
    """
    def chunk(tag, data):
        return struct.pack("!I", len(data)) + tag + data + struct.pack("!I", zlib.crc32(tag + data) & 0xffffffff)

    header = b"\x89PNG\r\n\x1a\n"
    # IHDR: width(4), height(4), bit_depth(1=8), color_type(1=2 RGB), compression(1=0), filter(1=0), interlace(1=0)
    ihdr = chunk(b"IHDR", struct.pack("!IIBBBBB", width, height, 8, 2, 0, 0, 0))
    idat = chunk(b"IDAT", zlib.compress(pixels, 9))
    iend = chunk(b"IEND", b"")

    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    with open(output_path, "wb") as f:
        f.write(header + ihdr + idat + iend)

def create_splash_pixels(width, height, is_small=False):
    # ActonOS Colors
    # Deep Ink: (19, 14, 48) - #130e30
    # Deep Ink Gradient Start: (32, 24, 75)
    # Hi-Yellow: (255, 226, 40) - #ffe228
    # Canvas White: (249, 251, 242)
    # Accent Cyan/Grid: (255, 226, 40)
    
    rows = []
    cx, cy = width // 2, (height // 3 if not is_small else height // 3)
    radius = 35 if not is_small else 20
    
    for y in range(height):
        row = bytearray([0]) # PNG filter byte 0 (None)
        y_dist = y - cy
        
        for x in range(width):
            x_dist = x - cx
            dist = math.sqrt(x_dist * x_dist + y_dist * y_dist)
            
            # Base background: smooth vertical and radial gradient
            grad_factor = min(1.0, math.sqrt((x / width - 0.5)**2 + (y / height - 0.5)**2) * 1.5)
            r = int(30 * (1 - grad_factor) + 12 * grad_factor)
            g = int(24 * (1 - grad_factor) + 8 * grad_factor)
            b = int(72 * (1 - grad_factor) + 26 * grad_factor)
            
            # Draw border frame
            margin = 40 if not is_small else 15
            if (x == margin or x == width - margin) and margin <= y <= height - margin:
                r, g, b = 255, 226, 40
            elif (y == margin or y == height - margin) and margin <= x <= width - margin:
                r, g, b = 255, 226, 40
                
            # Draw Logo Shield in Center
            if dist < radius:
                # Outer Shield
                r, g, b = 255, 226, 40 # Hi-Yellow
                if dist < radius - 6 and dist > 8:
                    # Inner Dark Core
                    r, g, b = 19, 14, 48
                elif dist <= 8:
                    # Center Yellow Dot
                    r, g, b = 255, 226, 40
            elif dist < radius + 12:
                # Outer Yellow Glow
                blend = (radius + 12 - dist) / 12.0
                r = int(r * (1 - blend) + 255 * blend)
                g = int(g * (1 - blend) + 226 * blend)
                b = int(b * (1 - blend) + 40 * blend)
                
            # Subtle grid lines
            if not is_small and (x % 96 == 0 or y % 96 == 0) and margin < x < width - margin and margin < y < height - margin:
                r = min(255, r + 15)
                g = min(255, g + 15)
                b = min(255, b + 25)
                
            row.extend([r, g, b])
            
        rows.append(bytes(row))
        
    return b"".join(rows)

def main():
    base_dir = os.path.dirname(os.path.abspath(__file__))
    live_build_dir = os.path.dirname(base_dir)
    
    print("Generating ActonOS Brand Splash Screens (Pure Python - No External Deps)...")
    
    # 1. 1080p Splash for GRUB
    pixels_1080p = create_splash_pixels(1920, 1080, is_small=False)
    splash_1080p = os.path.join(base_dir, "splash_1080p.png")
    write_png(splash_1080p, 1920, 1080, pixels_1080p)
    print(f"Generated: {splash_1080p}")
    
    # 2. 640x480 Splash for Syslinux / Legacy ISOLINUX
    pixels_640 = create_splash_pixels(640, 480, is_small=True)
    splash_640 = os.path.join(base_dir, "splash_isolinux.png")
    write_png(splash_640, 640, 480, pixels_640)
    print(f"Generated: {splash_640}")
    
    # 3. Deploy to bootloader folders
    targets = [
        (os.path.join(live_build_dir, "config", "bootloaders", "grub-pc", "splash.png"), 1920, 1080, pixels_1080p),
        (os.path.join(live_build_dir, "config", "bootloaders", "grub-efi", "splash.png"), 1920, 1080, pixels_1080p),
        (os.path.join(live_build_dir, "config", "bootloaders", "syslinux", "splash.png"), 640, 480, pixels_640),
        (os.path.join(live_build_dir, "config", "includes.binary", "isolinux", "splash.png"), 640, 480, pixels_640)
    ]
    for path, w, h, px in targets:
        write_png(path, w, h, px)
        print(f"Deployed: {path}")

if __name__ == "__main__":
    main()
