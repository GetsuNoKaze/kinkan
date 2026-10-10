#!/usr/bin/env python3
"""Keep GitHub's preferred .github README and Russian entrypoints in sync."""
from pathlib import Path
import re
root = Path(__file__).resolve().parents[2]
def github_copy(source):
    def relative(m):
        start, target, end = m.groups()
        if not target.startswith(('https://', 'http://', '#', 'mailto:')):
            target = '../' + target
        return start + target + end
    return re.sub(r'(\]\(|src="|srcset=")([^\s)"<>]+)(\)|")', relative, source)
ru = (root/'README.md').read_text()
(root/'README.ru.md').write_text(ru)
for filename in ['README.md','README.ru.md','README.en.md']:
    (root/'.github'/filename).write_text(github_copy((root/filename).read_text()))
