clnup
=====

A small command-line tool that deletes (or dry-run lists) files and directories matched by rules in a
``.clnup`` file — similar in spirit to ``.gitignore``, but for cleanup instead of exclusion.

Available in two implementations: **Zig** (``clnup.zig``, single binary, no runtime) and
**Go** (``clnup.go`` + platform shims).

Requires **Zig 0.16** or **Go 1.21+**.

----

Usage
-----

.. code-block:: text

    clnup [-r] [-f <file>] [-q] [-v] [-d] [-action print|delete|touch] [path]

    Options:
      -r            Recurse into subdirectories
      -f FILE       Rules file (default: .clnup)
      -q            Quiet — suppress normal output
      -v            Verbose — print extra logging
      -d            Dry run — print matches, delete nothing
      -action NAME  Handler: print | delete | touch  (default: delete, Go only)

``path`` defaults to ``.`` (current directory).

Rules file
----------

Each line is a glob pattern with an optional stat predicate after ``|``.
Blank lines and ``#`` comments are ignored. Last matching rule wins.

.. code-block:: text

    # glob only — identical to .gitignore syntax
    *.o
    /build/
    !important.o

    # glob + stat predicate
    *.log        | stat.size > 100mb
    *.tmp        | stat.mtime < now-14d
    /build/      | stat.uid == 1000
    cache/       | stat.size > 50mb || stat.atime < now-30d
    *.bin        | stat.size > 100mb && stat.size < 1gb

**Glob syntax**

.. list-table::
   :widths: 20 80
   :header-rows: 1

   * - Syntax
     - Meaning
   * - ``*.o``
     - Any entry named ``*.o`` at any depth (non-anchored)
   * - ``/build``
     - Top-level only (leading ``/`` anchors to root)
   * - ``target/``
     - Directories only (trailing ``/``)
   * - ``!keep.o``
     - Negate — keep this entry even if an earlier rule would delete it

**Stat predicate syntax**

.. code-block:: text

    stat.FIELD  OP  VALUE

    FIELD   size | mtime | atime | ctime | uid | gid | mode
    OP      <  <=  >  >=  ==  !=
    VALUE   integer         plain integer (bytes / nanoseconds / raw)
            Nkb Nmb Ngb    size suffix   (size field only)
            now-Nd          N days ago   (time fields)
            now-Nh          N hours ago
            now-Nm          N minutes ago

Multiple predicates combine with ``&&`` (higher precedence) and ``||``.
Platform note: ``uid``, ``gid``, and ``atime``/``ctime`` semantics vary by OS
(``ctime`` is metadata-change time on Linux, creation time on Windows).
Unsupported fields are silently skipped; the rule does not fire.

Building
--------

.. code-block:: sh

    # Zig
    zig build-exe clnup.zig -O ReleaseSafe

    # Go
    go build -o clnup .

Testing (Go)
------------

.. code-block:: sh

    go test ./...

Examples
--------

Dry-run, show what would be deleted recursively::

    clnup -r -d

Delete matched files in ``./dist``, quiet::

    clnup -q dist

Delete log files larger than 100 MB::

    echo '*.log | stat.size > 100mb' > .clnup
    clnup -r

Use a custom rules file::

    clnup -f .myclnup -r

License
-------

MIT License

Copyright (c) 2026

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
associated documentation files (the "Software"), to deal in the Software without restriction,
including without limitation the rights to use, copy, modify, merge, publish, distribute,
sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial
portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT
OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
