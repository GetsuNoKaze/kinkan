#!/usr/bin/env python3
"""Rebuild README SVGs and a video tour from the inherited demo screenshots.
Requires Python 3, ffmpeg with libx264, and a font containing Cyrillic.
Run from any directory: python3 scripts/readme/make-assets.py --font /path/font.ttf
"""
import argparse
from pathlib import Path
import subprocess
import tempfile
from xml.sax.saxutils import escape

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / '.github/assets/kinkan'
OUT.mkdir(parents=True, exist_ok=True)

def svg(body, height):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="{height}" viewBox="0 0 1280 {height}" role="img">{body}</svg>\n'

def text(x, y, content, size=22, color='#24363a', weight=400):
    return f'<text x="{x}" y="{y}" fill="{color}" font-family="Arial, sans-serif" font-size="{size}" font-weight="{weight}">{escape(content)}</text>'

for lang in ('ru', 'en'):
    for theme in ('light', 'dark'):
        dark = theme == 'dark'
        bg, ink, muted, panel = ('#101b22','#fff5e4','#b2c5cc','#20323b') if dark else ('#fff8ec','#24363a','#60767a','#ffffff')
        title = 'Свой сайт. Измерения. Меньше догадок.' if lang == 'ru' else 'Your site. Real measurements. Fewer guesses.'
        subtitle = 'VPN-панель на mihomo · неофициальный форк Mikan' if lang == 'ru' else 'VPN panel on mihomo · an unofficial Mikan fork'
        tags = ['Сайт ноды','Проверка протоколов','Журнал сканеров'] if lang == 'ru' else ['Node websites','Protocol probes','Scanner journal']
        body = f'<title>Kinkan — {escape(title)}</title><rect width="1280" height="360" rx="28" fill="{bg}"/><circle cx="1110" cy="90" r="180" fill="#f4ad33" opacity=".10"/>'
        body += '<ellipse cx="122" cy="166" rx="57" ry="77" transform="rotate(-18 122 166)" fill="#f3a529"/><ellipse cx="114" cy="163" rx="42" ry="64" transform="rotate(-18 114 163)" fill="#ffbf49"/><path d="M122 87 Q129 43 174 53 Q171 86 122 87" fill="#52886d"/>'
        body += text(225,105,'kinkan',66,ink,700)+text(225,160,title,29,ink,700)+text(225,204,subtitle,21,muted)
        for x,label in zip((225,490,755),tags):
            body += f'<rect x="{x}" y="248" width="246" height="48" rx="15" fill="{panel}"/>'+text(x+18,279,label,19,ink)
        (OUT/f'banner-{lang}-{theme}.svg').write_text(svg(body,360))
    words = ['Посторонний запрос','Нет верного пароля','Сайт ноды','Обычная страница','Проверка протоколов','Ответы → сравнение → отчёт'] if lang=='ru' else ['Unsolicited request','No valid password','Node website','An ordinary web page','Protocol probes','Responses → comparison → report']
    body='<title>Kinkan: TrustTunnel fallback and protocol probes</title><rect width="1280" height="340" rx="24" fill="#fff8ec"/><defs><marker id="arrow" markerWidth="10" markerHeight="10" refX="8" refY="5" orient="auto"><path d="M0 0 L10 5 L0 10" fill="#698581"/></marker></defs>'
    for x,title,subtitle in [(42,words[0],words[1]),(456,'TrustTunnel · fallback','Kinkan'),(870,words[2],words[3])]:
        body+=f'<rect x="{x}" y="42" width="368" height="120" rx="20" fill="white" stroke="#e3d9c5"/>'+text(x+24,90,title,23,weight=700)+text(x+24,129,subtitle,19,'#60767a')
    for a,b in [(410,447),(824,861)]:
        body+=f'<path d="M{a} 103 H{b}" stroke="#698581" stroke-width="3" marker-end="url(#arrow)"/>'
    body+='<rect x="42" y="204" width="1196" height="91" rx="20" fill="#20363e"/>'+text(70,244,words[4],23,'#ffcf76',700)+text(70,276,words[5],19,'#e5efef')
    (OUT/f'flow-{lang}.svg').write_text(svg(body,340))

parser=argparse.ArgumentParser()
parser.add_argument('--font',required=True)
args=parser.parse_args()
# This is a slideshow of demo screenshots, not a recording of a live deployment.
with tempfile.TemporaryDirectory(prefix='kinkan-readme-') as tmp:
    tmp=Path(tmp)
    inputs=[]
    for i,(name,title) in enumerate([('dashboard','Обзор панели'),('users','Пользователи и тарифы'),('inbounds','Подключения и протоколы'),('telegram','Telegram-бот')]):
        caption=tmp/f'{i}.txt';caption.write_text(title)
        filters=f"scale=1120:620:force_original_aspect_ratio=decrease,pad=1280:800:(ow-iw)/2:130:color=0x101b22,drawtext=fontfile='{args.font}':textfile='{caption}':fontcolor=0xffcf76:fontsize=32:x=80:y=44,drawtext=fontfile='{args.font}':text='Kinkan / Mikan — demo UI':fontcolor=0xb2c5cc:fontsize=18:x=80:y=92,format=yuv420p"
        part=tmp/f'{i}.mp4'
        subprocess.run(['ffmpeg','-y','-loglevel','error','-loop','1','-i',str(ROOT/f'.github/assets/screens/ru/{name}.webp'),'-vf',filters,'-t','4','-r','15','-c:v','libx264','-preset','medium','-crf','24',str(part)],check=True)
        inputs.append(f"file '{part}'")
    playlist=tmp/'parts.txt';playlist.write_text('\n'.join(inputs)+'\n')
    subprocess.run(['ffmpeg','-y','-loglevel','error','-f','concat','-safe','0','-i',str(playlist),'-c','copy','-movflags','+faststart',str(OUT/'tour-ru.mp4')],check=True)
    subprocess.run(['ffmpeg','-y','-loglevel','error','-i',str(OUT/'tour-ru.mp4'),'-filter_complex','fps=2,scale=640:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=bayer','-loop','0',str(OUT/'tour-ru.gif')],check=True)
