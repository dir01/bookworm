#!/usr/bin/env python3
"""Download English Project Gutenberg ZIM archives from the Kiwix mirror.

Standard library only.

It looks at the Kiwix Gutenberg directory listing and, for the English
(``gutenberg_en_*``) archives, downloads what you are missing according to a
few rules:

* English only (``gutenberg_en_all_*`` and ``gutenberg_en_lcc-*``).
* Only the newest version of each archive (e.g. ``lcc-p_2026-03`` supersedes
  ``lcc-p_2025-12``).
* Skip anything already present in the target directory.
* Skip an ``lcc-*`` split when a ``-all`` archive you already have (or are about
  to download in this run) is at least as new, because a full ``-all`` archive
  already contains that split's books. In other words: an ``lcc-*`` split is
  only worth downloading if it is *newer* than your newest ``-all``.

  (If you actually want the superseded splits too, pass ``--include-superseded``.)

Downloads are resumable: interrupted files land in ``<name>.zim.part`` and are
continued via an HTTP Range request on the next run.

Examples:
    # Preview what would be downloaded into the current directory:
    ./sync_gutenberg_en.py --dry-run

    # Download missing English archives into /data/zim:
    ./sync_gutenberg_en.py --dir /data/zim
"""

import argparse
import os
import re
import sys
import urllib.request

BASE_URL = "https://lb.download.kiwix.org/zim/gutenberg/"
LANG = "en"
USER_AGENT = "bookworm-gutenberg-sync/1.0"

# gutenberg_<lang>_<variant>_<YYYY-MM>.zim  where variant is "all" or "lcc-<x>"
NAME_RE = re.compile(r"^gutenberg_([a-z0-9-]+)_(all|lcc-[a-z]+)_(\d{4}-\d{2})\.zim$")


class Archive:
    __slots__ = ("filename", "lang", "variant", "date")

    def __init__(self, filename, lang, variant, date):
        self.filename = filename
        self.lang = lang
        self.variant = variant
        self.date = date  # "YYYY-MM"; lexicographic order == chronological order

    def __repr__(self):
        return self.filename


def parse_name(filename):
    m = NAME_RE.match(filename)
    if not m:
        return None
    return Archive(filename, m.group(1), m.group(2), m.group(3))


def http_get(url, timeout=60):
    req = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    return urllib.request.urlopen(req, timeout=timeout)


def list_remote(base_url, timeout=60):
    """Return {variant: Archive} keeping only the newest English archive per variant."""
    with http_get(base_url, timeout=timeout) as resp:
        html = resp.read().decode("utf-8", "replace")

    latest = {}
    for filename in re.findall(r'href="(gutenberg_[^"]+\.zim)"', html):
        arc = parse_name(filename)
        if arc is None or arc.lang != LANG:
            continue
        cur = latest.get(arc.variant)
        if cur is None or arc.date > cur.date:
            latest[arc.variant] = arc
    return latest


def scan_local(directory):
    """Scan the target directory for English archives already on disk.

    Return ``(present_filenames, local_newest)`` where ``local_newest`` maps each
    variant to the newest date present locally (so a remote archive can be
    recognised as superseded by a newer local copy of the same variant).
    """
    present = set()
    local_newest = {}
    try:
        entries = os.listdir(directory)
    except FileNotFoundError:
        entries = []
    for name in entries:
        arc = parse_name(name)
        if arc is None or arc.lang != LANG:
            continue
        present.add(name)
        cur = local_newest.get(arc.variant)
        if cur is None or arc.date > cur:
            local_newest[arc.variant] = arc.date
    return present, local_newest


def plan(remote, present, local_newest, include_superseded):
    """Yield (action, archive, reason) for every candidate, action in {download, skip}."""
    remote_all = remote.get("all")

    # The best -all we will have after this run: the newest of what's on disk and
    # the newest -all we're going to download.
    effective_all = local_newest.get("all")
    if remote_all is not None and (effective_all is None or remote_all.date > effective_all):
        effective_all = remote_all.date

    for variant in sorted(remote):
        arc = remote[variant]
        if arc.filename in present:
            yield "skip", arc, "already present"
            continue
        have_date = local_newest.get(variant)
        if have_date is not None and have_date >= arc.date:
            # A same-or-newer copy of this exact variant is already on disk.
            yield "skip", arc, "newer local copy (%s)" % have_date
            continue
        if (
            variant != "all"
            and not include_superseded
            and effective_all is not None
            and arc.date <= effective_all
        ):
            yield "skip", arc, "covered by newer -all (%s)" % effective_all
            continue
        yield "download", arc, "missing"


def human(nbytes):
    n = float(nbytes)
    for unit in ("B", "KiB", "MiB", "GiB", "TiB"):
        if n < 1024 or unit == "TiB":
            return "%.1f %s" % (n, unit)
        n /= 1024


def download(url, dst, timeout=60):
    """Download url to dst with resume support via a .part file."""
    part = dst + ".part"
    have = os.path.getsize(part) if os.path.exists(part) else 0

    headers = {"User-Agent": USER_AGENT}
    if have:
        headers["Range"] = "bytes=%d-" % have

    req = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        resuming = resp.status == 206 and resp.headers.get("Content-Range")
        if have and not resuming:
            # Server ignored our Range; start over from scratch.
            have = 0

        remaining = resp.headers.get("Content-Length")
        total = (have + int(remaining)) if remaining is not None else None

        mode = "ab" if have else "wb"
        downloaded = have
        with open(part, mode) as f:
            while True:
                chunk = resp.read(1024 * 256)
                if not chunk:
                    break
                f.write(chunk)
                downloaded += len(chunk)
                if total:
                    pct = 100.0 * downloaded / total
                    msg = "\r  %s / %s (%.1f%%)" % (human(downloaded), human(total), pct)
                else:
                    msg = "\r  %s" % human(downloaded)
                sys.stderr.write(msg)
                sys.stderr.flush()
    sys.stderr.write("\n")

    # If the server advertised a length but closed the connection early, read()
    # returns EOF without raising. Don't promote a truncated .part to a finished
    # .zim -- leave it in place so the next run resumes from where it stopped.
    if total is not None and downloaded != total:
        raise IOError(
            "incomplete download: got %s of %s; keeping %s for resume"
            % (human(downloaded), human(total), os.path.basename(part))
        )
    os.replace(part, dst)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--dir", default=os.getcwd(),
                        help="target directory for the archives (default: current dir)")
    parser.add_argument("--base-url", default=BASE_URL,
                        help="Kiwix Gutenberg directory URL")
    parser.add_argument("--dry-run", action="store_true",
                        help="print the plan without downloading anything")
    parser.add_argument("--include-superseded", action="store_true",
                        help="also download lcc-* splits older than a local -all "
                             "(by default those are skipped)")
    parser.add_argument("--timeout", type=int, default=60,
                        help="socket timeout in seconds (default: 60)")
    args = parser.parse_args(argv)

    try:
        remote = list_remote(args.base_url, timeout=args.timeout)
    except OSError as e:
        print("error: could not fetch listing from %s: %s" % (args.base_url, e), file=sys.stderr)
        return 1

    if not remote:
        print("no English archives found at %s" % args.base_url, file=sys.stderr)
        return 1

    present, local_newest = scan_local(args.dir)
    if local_newest.get("all"):
        print("newest local -all: %s" % local_newest["all"])

    to_download = []
    for action, arc, reason in plan(remote, present, local_newest, args.include_superseded):
        mark = "DOWNLOAD" if action == "download" else "skip    "
        print("%s  %-40s  %s" % (mark, arc.filename, reason))
        if action == "download":
            to_download.append(arc)

    if not to_download:
        print("\nnothing to download.")
        return 0

    if args.dry_run:
        print("\n%d archive(s) would be downloaded (dry run)." % len(to_download))
        return 0

    os.makedirs(args.dir, exist_ok=True)
    print("\ndownloading %d archive(s) into %s" % (len(to_download), args.dir))
    failures = 0
    for arc in to_download:
        url = args.base_url.rstrip("/") + "/" + arc.filename
        dst = os.path.join(args.dir, arc.filename)
        print("%s" % arc.filename)
        try:
            download(url, dst, timeout=args.timeout)
        except OSError as e:
            failures += 1
            print("  failed: %s" % e, file=sys.stderr)

    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
