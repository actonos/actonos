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

def load_rgba_png(path):
    """Decodes an RGBA PNG file into a 2D list of (r, g, b, a) tuples using pure Python."""
    if not os.path.exists(path):
        return None, 0, 0
        
    with open(path, 'rb') as f:
        data = f.read()
        
    pos = 8
    idat = b''
    w = h = 0
    while pos < len(data):
        length, chunk_type = struct.unpack('!I4s', data[pos:pos+8])
        pos += 8
        chunk_data = data[pos:pos+length]
        pos += length + 4
        if chunk_type == b'IHDR':
            w, h, bit_depth, color_type = struct.unpack('!IIBB', chunk_data[:10])
        elif chunk_type == b'IDAT':
            idat += chunk_data
        elif chunk_type == b'IEND':
            break
            
    raw = zlib.decompress(idat)
    stride = w * 4
    pixels = []
    prev_row = bytearray(stride)
    raw_pos = 0
    
    for y in range(h):
        filter_type = raw[raw_pos]
        raw_pos += 1
        curr_row = bytearray(raw[raw_pos:raw_pos+stride])
        raw_pos += stride
        recon = bytearray(stride)
        
        for x in range(stride):
            a = recon[x - 4] if x >= 4 else 0
            b = prev_row[x]
            c = prev_row[x - 4] if x >= 4 else 0
            if filter_type == 0:
                recon[x] = curr_row[x]
            elif filter_type == 1:
                recon[x] = (curr_row[x] + a) & 0xff
            elif filter_type == 2:
                recon[x] = (curr_row[x] + b) & 0xff
            elif filter_type == 3:
                recon[x] = (curr_row[x] + (a + b) // 2) & 0xff
            elif filter_type == 4:
                p = a + b - c
                pa = abs(p - a)
                pb = abs(p - b)
                pc = abs(p - c)
                pr = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
                recon[x] = (curr_row[x] + pr) & 0xff
        prev_row = recon
        
        row_pixels = []
        for x in range(0, stride, 4):
            row_pixels.append((recon[x], recon[x+1], recon[x+2], recon[x+3]))
        pixels.append(row_pixels)
        
    return pixels, w, h

def resize_rgba_image(src_pixels, src_w, src_h, dst_w, dst_h):
    """Bilinear scaling of RGBA image."""
    dst = []
    x_ratio = float(src_w - 1) / dst_w if dst_w > 0 else 0
    y_ratio = float(src_h - 1) / dst_h if dst_h > 0 else 0
    
    for i in range(dst_h):
        row = []
        y = int(y_ratio * i)
        y_diff = (y_ratio * i) - y
        y1 = min(y + 1, src_h - 1)
        
        for j in range(dst_w):
            x = int(x_ratio * j)
            x_diff = (x_ratio * j) - x
            x1 = min(x + 1, src_w - 1)
            
            p1 = src_pixels[y][x]
            p2 = src_pixels[y][x1]
            p3 = src_pixels[y1][x]
            p4 = src_pixels[y1][x1]
            
            r = int(p1[0]*(1-x_diff)*(1-y_diff) + p2[0]*x_diff*(1-y_diff) + p3[0]*y_diff*(1-x_diff) + p4[0]*x_diff*y_diff)
            g = int(p1[1]*(1-x_diff)*(1-y_diff) + p2[1]*x_diff*(1-y_diff) + p3[1]*y_diff*(1-x_diff) + p4[1]*x_diff*y_diff)
            b = int(p1[2]*(1-x_diff)*(1-y_diff) + p2[2]*x_diff*(1-y_diff) + p3[2]*y_diff*(1-x_diff) + p4[2]*x_diff*y_diff)
            a = int(p1[3]*(1-x_diff)*(1-y_diff) + p2[3]*x_diff*(1-y_diff) + p3[3]*y_diff*(1-x_diff) + p4[3]*x_diff*y_diff)
            
            row.append((min(255, max(0, r)), min(255, max(0, g)), min(255, max(0, b)), min(255, max(0, a))))
        dst.append(row)
    return dst

# ==============================================================================
# ActonOS Design System Tokens (from docs/DESIGN.md)
# ==============================================================================
COLOR_DEEP_INK     = (19, 14, 48)     # #130e30 - Primary brand ink
COLOR_HI_YELLOW    = (255, 226, 40)   # #ffe228 - Primary action & highlight
COLOR_MOSS_GREEN   = (89, 226, 93)    # #59e25d - Decorative ambient blob
COLOR_FUCHSIA      = (226, 97, 229)   # #e261e5 - Decorative ambient blob
COLOR_CANVAS       = (249, 251, 242)  # #f9fbf2 - Page background / clean surface
COLOR_SLATE        = (95, 92, 110)    # #5f5c6e - Muted text & subtle borders

FONT_5X7 = {
    'A': [0x0C, 0x12, 0x12, 0x1E, 0x12, 0x12, 0x00],
    'C': [0x0E, 0x10, 0x10, 0x10, 0x10, 0x0E, 0x00],
    'T': [0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x00],
    'O': [0x0E, 0x11, 0x11, 0x11, 0x11, 0x0E, 0x00],
    'N': [0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x00],
    'S': [0x0E, 0x10, 0x0E, 0x01, 0x01, 0x1E, 0x00],
    'E': [0x1F, 0x10, 0x1E, 0x10, 0x10, 0x1F, 0x00],
    'X': [0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11, 0x00],
    'I': [0x0E, 0x04, 0x04, 0x04, 0x04, 0x0E, 0x00],
    'B': [0x1E, 0x11, 0x1E, 0x11, 0x11, 0x1E, 0x00],
    'L': [0x10, 0x10, 0x10, 0x10, 0x10, 0x1F, 0x00],
    'P': [0x1E, 0x11, 0x1E, 0x10, 0x10, 0x10, 0x00],
    'R': [0x1E, 0x11, 0x1E, 0x14, 0x12, 0x11, 0x00],
    'G': [0x0E, 0x10, 0x13, 0x11, 0x11, 0x0E, 0x00],
    'U': [0x11, 0x11, 0x11, 0x11, 0x11, 0x0E, 0x00],
    'D': [0x1C, 0x12, 0x11, 0x11, 0x12, 0x1C, 0x00],
    'K': [0x11, 0x12, 0x1C, 0x12, 0x11, 0x11, 0x00],
    'M': [0x11, 0x1B, 0x15, 0x11, 0x11, 0x11, 0x00],
    'V': [0x11, 0x11, 0x11, 0x0A, 0x04, 0x04, 0x00],
    '1': [0x04, 0x0C, 0x04, 0x04, 0x04, 0x0E, 0x00],
    '0': [0x0E, 0x11, 0x13, 0x15, 0x19, 0x0E, 0x00],
    '.': [0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C],
    '-': [0x00, 0x00, 0x1F, 0x00, 0x00, 0x00, 0x00],
    ' ': [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]
}

def render_bitmap_text(canvas, text, start_x, start_y, scale, color, max_w, max_h):
    cur_x = start_x
    for char in text.upper():
        bitmap = FONT_5X7.get(char, FONT_5X7[' '])
        for row_idx, row_byte in enumerate(bitmap):
            for col_idx in range(5):
                if (row_byte >> (4 - col_idx)) & 1:
                    for dy in range(scale):
                        for dx in range(scale):
                            px = cur_x + col_idx * scale + dx
                            py = start_y + row_idx * scale + dy
                            if 0 <= px < max_w and 0 <= py < max_h:
                                canvas[py][px] = color
        cur_x += 6 * scale

def create_design_system_splash(width, height, icon_pixels, icon_w, icon_h, is_small=False):
    canvas = [[COLOR_DEEP_INK for _ in range(width)] for _ in range(height)]
    
    # 1. Ambient Backdrop Blobs (DESIGN.md tokens)
    g1_x, g1_y = int(width * 0.22), int(height * 0.28)
    g1_radius = int(width * 0.35)
    g2_x, g2_y = int(width * 0.78), int(height * 0.25)
    g2_radius = int(width * 0.32)
    g3_x, g3_y = int(width * 0.50), int(height * 0.40)
    g3_radius = int(width * 0.25)
    
    for y in range(height):
        for x in range(width):
            r, g, b = COLOR_DEEP_INK
            d1 = math.sqrt((x - g1_x)**2 + (y - g1_y)**2)
            d2 = math.sqrt((x - g2_x)**2 + (y - g2_y)**2)
            d3 = math.sqrt((x - g3_x)**2 + (y - g3_y)**2)
            
            if d1 < g1_radius:
                alpha1 = (1.0 - (d1 / g1_radius)) ** 2 * 0.14
                r = int(r * (1 - alpha1) + COLOR_MOSS_GREEN[0] * alpha1)
                g = int(g * (1 - alpha1) + COLOR_MOSS_GREEN[1] * alpha1)
                b = int(b * (1 - alpha1) + COLOR_MOSS_GREEN[2] * alpha1)
                
            if d2 < g2_radius:
                alpha2 = (1.0 - (d2 / g2_radius)) ** 2 * 0.12
                r = int(r * (1 - alpha2) + COLOR_FUCHSIA[0] * alpha2)
                g = int(g * (1 - alpha2) + COLOR_FUCHSIA[1] * alpha2)
                b = int(b * (1 - alpha2) + COLOR_FUCHSIA[2] * alpha2)
                
            if d3 < g3_radius:
                alpha3 = (1.0 - (d3 / g3_radius)) ** 2 * 0.10
                r = int(r * (1 - alpha3) + COLOR_HI_YELLOW[0] * alpha3)
                g = int(g * (1 - alpha3) + COLOR_HI_YELLOW[1] * alpha3)
                b = int(b * (1 - alpha3) + COLOR_HI_YELLOW[2] * alpha3)
                
            canvas[y][x] = (min(255, r), min(255, g), min(255, b))
            
    # 2. Outer Border Frame
    margin = 32 if not is_small else 12
    for x in range(margin, width - margin):
        canvas[margin][x] = (35, 28, 75)
        canvas[height - margin][x] = (35, 28, 75)
    for y in range(margin, height - margin):
        canvas[y][margin] = (35, 28, 75)
        canvas[y][width - margin] = (35, 28, 75)
        
    # 3. Draw Official Brand Logo Icon (Replacing old yellow dot/shield)
    cx = width // 2
    cy = int(height * 0.26) if not is_small else int(height * 0.22)
    
    if icon_pixels:
        target_size = 140 if not is_small else 70
        scaled_icon = resize_rgba_image(icon_pixels, icon_w, icon_h, target_size, target_size)
        start_ix = cx - target_size // 2
        start_iy = cy - target_size // 2
        
        for iy in range(target_size):
            for ix in range(target_size):
                px = start_ix + ix
                py = start_iy + iy
                if 0 <= px < width and 0 <= py < height:
                    ir, ig, ib, ia = scaled_icon[iy][ix]
                    if ia > 0:
                        alpha = ia / 255.0
                        bg_r, bg_g, bg_b = canvas[py][px]
                        # Alpha blend official icon onto Deep Ink canvas
                        nr = int(bg_r * (1.0 - alpha) + ir * alpha)
                        ng = int(bg_g * (1.0 - alpha) + ig * alpha)
                        nb = int(bg_b * (1.0 - alpha) + ib * alpha)
                        canvas[py][px] = (nr, ng, nb)
                        
    # 4. Brand Typography
    # Title: ACTONOS (Hi-Yellow #ffe228)
    title_scale = 8 if not is_small else 4
    title_text = "ACTONOS"
    title_w = len(title_text) * 6 * title_scale
    title_x = cx - title_w // 2
    title_y = cy + (80 if not is_small else 45)
    render_bitmap_text(canvas, title_text, title_x, title_y, title_scale, COLOR_HI_YELLOW, width, height)
    
    # Subtitle: EXTENSIBLE AI AGENT OS (Canvas White #f9fbf2)
    if not is_small:
        sub_scale = 3
        sub_text = "EXTENSIBLE AI AGENT OS"
        sub_w = len(sub_text) * 6 * sub_scale
        sub_x = cx - sub_w // 2
        sub_y = title_y + 8 * title_scale + 12
        render_bitmap_text(canvas, sub_text, sub_x, sub_y, sub_scale, COLOR_CANVAS, width, height)
        
        # Pill Tag: AUTONOMOUS - DISTRIBUTED
        tag_scale = 2
        tag_text = "AUTONOMOUS - SANDBOXED - DISTRIBUTED"
        tag_w = len(tag_text) * 6 * tag_scale
        tag_x = cx - tag_w // 2
        tag_y = sub_y + 8 * sub_scale + 16
        
        pill_pad_x = 24
        pill_pad_y = 8
        p_left = tag_x - pill_pad_x
        p_right = tag_x + tag_w + pill_pad_x
        p_top = tag_y - pill_pad_y
        p_bottom = tag_y + 7 * tag_scale + pill_pad_y
        p_h = p_bottom - p_top
        
        for py in range(p_top, p_bottom):
            for px in range(p_left, p_right):
                if px < p_left + p_h // 2:
                    if (px - (p_left + p_h // 2))**2 + (py - (p_top + p_h // 2))**2 > (p_h // 2)**2:
                        continue
                elif px > p_right - p_h // 2:
                    if (px - (p_right - p_h // 2))**2 + (py - (p_top + p_h // 2))**2 > (p_h // 2)**2:
                        continue
                canvas[py][px] = (30, 24, 68)
                
        render_bitmap_text(canvas, tag_text, tag_x, tag_y, tag_scale, COLOR_HI_YELLOW, width, height)

    # 5. Bottom Status
    if not is_small:
        foot_scale = 2
        foot_text = "V1.0 APPLIANCE - PRESS ENTER TO BOOT"
        foot_w = len(foot_text) * 6 * foot_scale
        foot_x = cx - foot_w // 2
        foot_y = height - margin - 36
        render_bitmap_text(canvas, foot_text, foot_x, foot_y, foot_scale, COLOR_SLATE, width, height)

    rows = []
    for y in range(height):
        row = bytearray([0])
        for x in range(width):
            row.extend(canvas[y][x])
        rows.append(bytes(row))
        
    return b"".join(rows)

def main():
    base_dir = os.path.dirname(os.path.abspath(__file__))
    live_build_dir = os.path.dirname(base_dir)
    root_dir = os.path.dirname(os.path.dirname(live_build_dir))
    icon_path = os.path.join(root_dir, "web", "public", "actonos_icon.png")
    
    print(f"Loading official brand icon from: {icon_path}")
    icon_pixels, icon_w, icon_h = load_rgba_png(icon_path)
    
    print("Generating ActonOS Official Logo Splash Screens...")
    
    # 1. 1080p Splash for GRUB Bootloader
    pixels_1080p = create_design_system_splash(1920, 1080, icon_pixels, icon_w, icon_h, is_small=False)
    splash_1080p = os.path.join(base_dir, "splash_1080p.png")
    write_png(splash_1080p, 1920, 1080, pixels_1080p)
    print(f"Generated: {splash_1080p}")
    
    # 2. 640x480 Splash for Syslinux / ISOLINUX
    pixels_640 = create_design_system_splash(640, 480, icon_pixels, icon_w, icon_h, is_small=True)
    splash_640 = os.path.join(base_dir, "splash_isolinux.png")
    write_png(splash_640, 640, 480, pixels_640)
    print(f"Generated: {splash_640}")
    
    # 3. Deploy to bootloader directories
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
