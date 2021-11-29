_This whole design is still subject to radical changes. Until the
first tape is written for keeps, everything is up for grabs._

# Motivation

## Computer backup is not WORM backup

Most backup software these days aims to keep an incremental history
over time of many small files, stored as efficiently as possible. This
usually involves some form of deduplication using rolling hashes, as
found in Restic, Borg and Bup.

This is a pretty great solution for backing up your home directory, or
important files on a server. Slightly modified, it's also a decent
strategy for block-level backups such as those found in virtual
machine management software.

Systems based on these principles make a few core assumptions:

 - **Storage is expensive**; therefore effort should be expended to
   minimize the size of the backup set.
 - **Data will change over time**, and the deltas will be small;
   therefore effort should be expended to minimize the overhead of
   storing N small deltas.
 - **Random access is cheap**, and getting cheaper (compare spinning
   disks to SSDs); therefore storing a file non-linearly in exchange
   for reduced overhead (e.g. by chunking it into a content-addressed
   store) has minimal cost compared to the benefits.
 - **The dataset is (relatively) small**, likely at most a few GiB,
   and thus can be cheaply stored on any number of clouds or local
   media.

Now, say that instead of a typical home directory, you have a
collection of WORM (Write-Once Read-Many) data. For example,
phootographers with packrat tendencies accumulate large amounts of
heavy raw photo files. It's a similar story for video-enthusiastic
people, e.g. youtubers who want to preserve raw footage against future
need. Additionally, say that you have an LTO tape drive, or even
fancier a robot library.

All the assumptions above are incorrect when applied to this kind of
data, stored on tape:

 - **Storage is cheap**: LTO tape media has hovered around $7-10/TiB
   for decades, and it's not hard to find promotions as low as
   $3/TiB. Compare to $20/TiB for NAS-grade hard drives,
   $24.6/TiB/month stored + $92/TiB read for AWS S3, or
   ~$5/TiB/month + $10/TiB read for the cheaper cloud options.
 - **Data is largely unchanging**: once a photo or video file has been
   written out once, it's likely to never change again. New derived
   files (RAW to JPEG, transcodes to lower resolution) may be created,
   but the next major write event in the file's life is probably
   deletion.
 - **Random access is expensive**: each seek on a tape can take
   several _minutes_, even though sequential I/O can run at hundreds
   of MiB/s. It's even worse if you need to access a different tape:
   spooling and ejecting the currently loaded tape, loading a new tape
   and seeking on it can add up to 10 minutes or more, and that's if
   the tape is readily available in the robotic library.
 - **The dataset is huge**: even modest amounts of video will promptly
   reach into the hundred of GiB without trying too hard. "look at my
   shameful pile of looseleaf hard drives full of content" is a
   popular type of video for professional youtubers.

Conclusion: tape backup of WORM data requires a solution that
isn't well served by standard backup software.

## Tape backup software is built for CERN, not for you

Tape-aware backup software is designed for use at massive enterprise
scale: hundreds of data sources, streaming backups to dozens of tape
drives in large robots, with exquisitely complicated schedules for
taking, expiring, storing, consolidating and verifying backups.

I'm sure this software works very well for the likes of CERN, but I
have 1 NAS full of bytes, 1 tape drive, and I want to move A to
B. Existing software doesn't scale down to that.

When I first got my tape drive, I tried setting up both Bacula and
Bareos, the two flagship open-source tape aware software suites. Each
took me several _days_ of reading manuals to configure, required
running 3 daemons and a database server on my single computer to do
anything, were brittle and hard to monitor, and were unable to
saturate my tape drive's write bandwidth (a measly 160MiB/s - my ZFS
setup can read at over 500MiB/s sustained).

Conclusion: smaller scale backup to tape is an underserved niche. As
far as I can tell, the people who attempt it end up eihter suffering
through one of the enterprise stacks, or just format the tapes as LTFS
and treat them as a weirdly-shaped hard drive whose content they have
to curate by hand.

## Tape backup software is built for people who don't lose stuff

The tape-aware software suites write to tapes in a custom format that
cannot be read back out "naively" using basic tools. This means that a
"found tape" from the back of your closet is all but useless: if it's
old enough, the software has moved on and can no longer read its own
older format, and you're left with an undifferentiated stream of bytes
to reverse engineer.

And even if you're lucky on that front, individual tapes in these
systems are not usually "standalone": you need an out of band
"catalog" to tell you what bytes the tapes contain, and where to find
the files you care about.

This isn't universal, some of these tools have recovery software that
can painstakingly reconstruct catalog information from the raw
data. The ones I've seen do this by reading out the entire contents of
every tape you can give it, which takes 12-16h per tape if you can
keep the read throughput up.

I think it's fair to say that the solutions that do offer low-level
disaster recovery expect you to never have to use them, and instead
assume you'll be able to host an extremely highly available and
resilient catalog "somewhere else" for years or decades. Enterprises
and large research labs are readily capable of such continuity across
years, but I don't trust myself to still have access to my Bacula
catalog in 2035.

Conclusion: right now, I can't write backups to tape and trust that
I'll be able to use them easily in 10 years.

# Goals

So, that's why I'm writing my own backup software. My goals:

 - Optimized for WORM data that doesn't deduplicate. Whole files are
   the finest level of granularity available.
 - Optimized for tape media. Full restores should spend >99% of their
   time in sequential reads, restoring a single file should require no
   more than 1 or 2 seeks.
 - Optimized for recoverability. Each tape should be self-describing
   as much as possible. A reasonably technical Unix knower should be
   able to restore files from a tape with no prior knowledge of the
   on-tape format.
 - Optimized for low "continuity of care". Failing to maintain a
   catalog database for 10 years should result in minor inconvenience
   at most. Given the software and a trained operator, recovery from
   catalog loss should take no more than 30 minutes per tape,
   including the latency of a robot library manipulating the tape.
 - File-oriented policymaking. Backup sets, backup jobs and so forth
   are implementation details, what I care about is that I have N
   independent copies of a particular file.
 - Built in verification support. This shouldn't deserve mention, but
   it's surprisingly uncommon at the low end to actually test
   backups. The software should make it as easy as possible.
 - Optimized for reasonably modern tape hardware. The goal is mass
   storage, so I'm not going to make it work for DAT or 8-track. The
   loose aim is that things LTO-5-ish drives can do will be enough.

In addition, as a secondary goal, it'd be nice to be able to use this
software on hard drives as well, treating them like "weird tapes". I
have a bunch of older drives lying around that could get a second life
as backup storage, with suitable respect paid to the lower expected
longevity of an old unplugged drive sitting on a shelf.

# Aside: the tape I/O interface

Tape exposes an API that is almost, but not quite entirely unlike
other modern storage. It's stream- and record-oriented storage, with
explicit seeking of the hardware read/write head, and a few other
things like filemarks which straight up don't exist elsewhere.

You talk to a tape drive with SCSI commands, same as everything these
days. When the tape is first loaded, the drive head is positioned at
the Beginning of Tape (BOT) well-known location. Reading and writing
is done in "records", which are blocks of bytes typically somewhere
between 512b and 4MiB.

The record size is left entirely to the user. Every read returns 1
record and advances the tape to the next, even if you provided a
target buffer that's too small for the record that was read. If you
issue a read with a 512b buffer, and the next record on the tape is
1MiB, you'll get the first 512b and a "short read" flag. The next
read will _not_ give you the remainder of that short read, instead
you'll receive the bytes of the next record. It's your problem to know
what record size is in use, and provide appropriately sized buffers.

In addition to data blocks, you can write out "file marks", which are
a physical manifestation of an EOF. If you write out two tarballs
separated by a file mark, trying to `dd` from the tape drive device
will yield the first tarball and stop when it hits the
filemark. Running `dd` again will yield the second tarball.

Drives can be told to seek to a record number (slowly, because the
record size and physical layout on the tape isn't as simple as
"multiply #records by length of 1 record and spool that many meters of
tape"), to a file mark (faster, because file marks are designed to be
"highly visible" even when the drive is spooling at high speed), to
BOT (IOW, full rewing), or to End of Media (EOM) which is the logical
"end of data you've written", and isn't related to the physical End of
Tape (EOT). In general, you can't go spelunking around in the space
between EOM and EOT, the best you can do is seek to EOM and write more
stuff.

Tapes have a nominal capacity written on them. For example, LTO-6
lists 2.5TiB per tape. However, tape is a fickle medium, and as a
result 1 byte of data may not always take the same amount of tape to
store. To deal with this, cartridges include a bit more physical tape
than strictly necessary, to account for space lost to manufacturing
defects or other issues with the medium. IOW, the nominal capacity is
a _minimum_, but you may be able to get a bit more data onto the tape
past that figure. Of a sample of a half-dozen brand new LTO-6 tapes, I
got anywhere from 1GiB to 50GiB "bonus" space at the end of the tape.

To account for this variable storage size, drives emit "early warning"
notifications when the drive is getting close to EOT. Starting in
LTO-5, the application can also move the warning zone earlier in the
tape, effectively being told ahead of time when the tape is down to
N-ish bytes of remaining space (-ish because see above about the
variability of the medium).

# Design overview: file format

The on-tape file format uses well-known, well-documented, open source
stuff only. Where possible, the most mature software possible, to
ensure that recovery doesn't depend on that one npm package that went
away in the Great Javascript Wars of 2031.

To that end, the on-tape format uses 3 basic building blocks:

 - [tar](https://en.wikipedia.org/wiki/Tar_(computing)), the venerable
   archive format literally designed for tape.
 - [sqlite](https://www.sqlite.org), a ubiquitous piece of software
   with _really_ [long-term support](https://www.sqlite.org/lts.html).
 - [age](https://github.com/FiloSottile/age), simple authenticated
   encryption for files, that isn't GPG.

```
+--------------------------------+
|                                |
|        Archaeology .tar        |
|                                |
+--------------------------------+
|             EOF                |
+--------------------------------+
|                                |
|       Index 1 sqlite DB        |
|                                |
+--------------------------------+
|             EOF                |
+--------------------------------+
|                                |
.        Archive 1 .tar          .
.                                .
.                                .
|                                |
+--------------------------------+
|             EOF                |
+--------------------------------+
|                                |
|       Index 2 sqlite DB        |
|                                |
+--------------------------------+
|             EOF                |
+--------------------------------+
|                                |
.        Archive 2 .tar          .
.                                .
.                                .
|                                |
+--------------------------------+
|             EOF                |
+--------------------------------+
|                                |
|     Empty index sqlite DB      |
|                                |
+--------------------------------+
|             EOF                |
+--------------------------------+
```

The main event on a tape is an uncompressed tar file containing the
files being backed up. If the data being backed up is compressible and
compression is desired, compression is applied to invidual files
within the archive. Initial versions will not support compression,
since WORM-worthy data tends to compress poorly or be already
compressed anyway.

Tar's big downside is that it's not indexed, so to find a single file
you have to scrub through the entire archive. To avoid that, the tar
file is preceded on the tape by a sqlite database file that provides
an index of the files in the tar archive. Given this database, you can
trivially look up the start record + length of any file in the
archive.

The index and archive are written out as separate files on the tape,
i.e. they are separated by a file mark. This simplifies readout with
non-specialized software: just issue two `dd`s back to back, and
you'll get 2 files out, one that file(1) will identify as a sqlite
database, and another that will identify as a tar file. Even if you
have no clue what these files are, sqlite databases are
self-describing (load them into the `sqlite3` tool and run `.schema`),
and tools to inspect and unpack tar files are ridiculously ubiquitous.

Both the index and archive are stored encrypted by age. age ciphertext
clearly advertises itself as such with a plaintext header, so even
with an encryption layer the "kind of file I have to deal with" can be
found out by trivial inspection (and presumably file(1) will someday
learn to recognize age ciphertext, making it even easier).

In cases where individual backup jobs don't fill a tape, this
index+tar file pair may be repeated on the tape, so successive `dd`s
will yield alternating index and archive files until EOM is reached.

In cases where a backup job is larger than the tape, the software
breaks the job down into N (index, archive) pairs that individually
fit on each tape. There is no support for "tape continuations", where
an archive stops partway and continues on a different tape. This may
lead to sub-optimal tape utilization if the files available for
bin-packing don't stack neatly into tape-sized chunks. In return, each
tape is an island that can be processed without knowledge of others,
there's never an archive without an associated index, and readers
don't have to recognize and handle a custom "file continues elsewhere"
mark on the tape.

For reasons explained in the next section, at the very end of the
tape, after the last pair of (index, archive) files, one final "empty"
index is present as the final file on the tape.

There are still a few bits of information missing for someone trying
to recover a tape with no prior knowledge of its format: what record
size was used to write the tape? Where's this "age" and "sqlite"
software you speak of? If said software is lost, what encoding and
algorithms did they use?

To address that, each tape begins with an unencrypted, uncompressed
tar file, whose contents identifies the version of this format that
was used to write the tape (so our software can quickly find out what
read procedure to use), and bundles a bunch of "archaeology" data to
teach a future reader how to read the rest of the tape. For example,
the archive could contain:

 - A text file describing the on-tape format.
 - Text files describing the file formats of tar, sqlite, and age.
 - Text files describing the algorithms used by age (STREAM with
   ChaCha20-Poly1305, X25519, scrypt, etc.).
 - The backup program binary that wrote the tape.
 - The corresponding source code of the backup program, including
   transitive dependencies.
 - A copy of the `sqlite3` binary and corresponding source code.

Compared to the overall capacity of a tape, this archive is tiny, and
the "waste" can easily be justified.

# Design overview: software

The backup program's very basic: you give it a bunch of roots to
scan. It finds all files within, groups them into tape-sized bundles
in the format above, and writes them out to tape. It persists the
files it's written, and the tapes it's written to, in a sqlite catalog
database.

From this catalog, it can offer some reporting on which files are
backed up, which versions (in case of WORM data that turned out to be
more Write-Seldom than Write-Once), where they are (which tape and
what location on tape), and so forth.

To mitigate the impact of losing the catalog, the entire catalog is
copied into every index file that's written out to tape. That is, the
index file on tape is a database that contains both an index of the
following archive on tape, and a full copy of the catalog as it was
right before the index file was written out.

Additionally, a full copy of the catalog is written out to the end of
full tapes (this is the "empty" index file alluded to
previously). This way, recovery from catalog loss is very easy: load
the last-used tape, seek to the last index on the tape, read it out,
and discard any actual index data within. What remains is a copy of
the catalog that describes everything as it was at the end of the last
backup job.

Similarly, even if the tapes are scattered far and wide, a single tape
becomes fully described in at most one sequential read, because the
last index on the tape contains the catalog up to but not including
the archive that (optionally) follows, and an index of the archive
that (optionally) follows.

In addition to fully describing itself, the recovered catalog also
describes all the tapes and backed-up data that predate the writing of
the tape in hand, so with every new tape found you recover much more
than the minimum necessary metadata, hopefully speeding up further
recovery.

Empirically, the catalog database is quite small. Even with all the
redundant copies, the catalogs should occupy much less than 1GiB of
each tape - 0.04% in the case of LTO-6.
