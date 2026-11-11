#!/usr/bin/env python3
"""Generate CheatChess's original sound set (wooden piece clacks + soft marimba cues).

Everything is synthesized from scratch (modal synthesis: a short noise "tap" exciting a few
exponentially-decaying wooden resonances), so there are no third-party assets or licenses.

    python3 frontend/sounds-src/generate.py      # writes frontend/public/sounds/*.mp3 (needs numpy + ffmpeg)

Tweak the numbers below and re-run to change the character of the sounds.
"""
import os
import subprocess
import tempfile
import wave

import numpy as np

SR = 48000
OUT = os.path.join(os.path.dirname(__file__), "..", "public", "sounds")
rng = np.random.default_rng(7)


def t(dur):
    return np.arange(int(dur * SR)) / SR


def wood(pitch=1.0, dur=0.16, bright=1.0, thump=1.0):
    """One piece landing on a wooden board."""
    x = t(dur)
    # (freq Hz, decay s, amplitude): piece + board resonances
    modes = [
        (190, 0.045, 0.30 * thump),  # board body thump
        (430, 0.030, 0.40),
        (840, 0.020, 1.00),  # main "tock"
        (1480, 0.013, 0.65),
        (2380, 0.008, 0.45 * bright),
        (3950, 0.004, 0.28 * bright),
    ]
    y = np.zeros_like(x)
    for f, tau, a in modes:
        f *= pitch * (1 + rng.uniform(-0.015, 0.015))
        y += a * np.exp(-x / tau) * np.sin(2 * np.pi * f * x + rng.uniform(0, 6.28))
    # contact transient: very short high-passed noise burst
    n = rng.standard_normal(len(x))
    n = np.diff(n, prepend=0) * np.exp(-x / 0.0012) * 0.55 * bright
    y += n
    y *= np.minimum(1, x / 0.0004)  # 0.4 ms attack, avoids a digital click
    return y


def marimba(freq, dur=0.6, amp=1.0):
    """Soft wooden bar tone (marimba-like partial ratios 1 : 4 : 10)."""
    x = t(dur)
    y = (np.exp(-x / 0.30) * np.sin(2 * np.pi * freq * x)
         + 0.25 * np.exp(-x / 0.06) * np.sin(2 * np.pi * freq * 4.0 * x)
         + 0.08 * np.exp(-x / 0.02) * np.sin(2 * np.pi * freq * 10.0 * x))
    y *= np.minimum(1, x / 0.002)
    return amp * y


def ping(freq, dur=0.35, tau=0.12, amp=1.0):
    x = t(dur)
    return amp * np.exp(-x / tau) * np.sin(2 * np.pi * freq * x) * np.minimum(1, x / 0.002)


def mix(parts, total):
    """parts: [(start_seconds, signal)]"""
    out = np.zeros(int(total * SR))
    for start, sig in parts:
        i = int(start * SR)
        n = min(len(sig), len(out) - i)
        out[i:i + n] += sig[:n]
    return out


def save(name, y, peak_db=-3.0):
    y = y / (np.max(np.abs(y)) + 1e-9) * 10 ** (peak_db / 20)
    pcm = (y * 32767).astype(np.int16)
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        tmp = f.name
    with wave.open(tmp, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(SR)
        w.writeframes(pcm.tobytes())
    dst = os.path.join(OUT, name + ".mp3")
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", tmp, "-b:a", "96k", dst], check=True)
    os.remove(tmp)
    print(f"{name:10s} {len(y) / SR:5.2f}s  {os.path.getsize(dst):6d} B")


os.makedirs(OUT, exist_ok=True)

# three slightly different moves so repeated moves don't sound robotic
for i, p in enumerate([1.0, 0.97, 1.03], 1):
    save(f"move{i}", wood(pitch=p), peak_db=-4)

# capture: piece knocked off (sharp, higher) + capturing piece lands (heavier)
save("capture", mix([(0, wood(pitch=1.18, dur=0.08, bright=1.3, thump=0.4) * 0.7),
                     (0.03, wood(pitch=0.9, thump=1.6))], 0.2), peak_db=-2)

# castle: king, then rook
save("castle", mix([(0, wood(pitch=1.0)), (0.11, wood(pitch=0.94) * 0.85)], 0.28), peak_db=-4)

# check: a firm clack plus a short bright warning ping
save("check", mix([(0, wood(pitch=0.95, thump=1.3)), (0.02, ping(1568, tau=0.09, amp=0.22))], 0.36), peak_db=-3)

# game start: rising G4 -> C5, game end: falling C5 -> G4
save("start", mix([(0, marimba(392.0)), (0.13, marimba(523.25))], 0.8), peak_db=-6)
save("end", mix([(0, marimba(523.25)), (0.15, marimba(392.0)), (0.30, marimba(261.63, amp=0.8))], 1.0), peak_db=-6)

# low time: two quick clock ticks
tick = wood(pitch=2.6, dur=0.04, bright=1.4, thump=0.1)
save("lowtime", mix([(0, tick), (0.12, tick)], 0.18), peak_db=-8)

# incoming challenge / notification: quick E5 -> A5
save("notify", mix([(0, marimba(659.25, 0.45)), (0.09, marimba(880.0, 0.5))], 0.62), peak_db=-7)
