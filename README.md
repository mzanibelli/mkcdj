# MkCDJ

A tool to manage an audio playlist with BPM and quality analysis.

## Dependencies

You need to have `ffmpeg(1)`, `sox(1)` and `bpm-tools` installed.
Ultimately, native implementations or low-level bindings would replace shell commands.

## Usage

- Run `mkcdj analyze PRESET PATH` to add a track to the collection
- Run `mkcdj compile PATH` to export all files to the given directory after converting them to a suitable format
- Run `mkcdj list` to preview the tracklist, with BPM and quality score
- Run `mkcdj prune` to remove lost files from the current playlist

## Configuration

The `MKCDJ_STORE` environment variable contains the path to the current collection (a JSON file).
If unset, `/tmp/mkcdj.json` is used.

## Presets

A preset is a shorthand to hint the BPM detection. Each preset limits the detection to its predefined BPM range.
For example, the `dnb` (Drum & Bass) preset limits the detection from 165 to 180 BPM.

## Quality

The quality score is computed from the average RMS gain over 20Khz relatively to the same measurement made over 16Khz.
Thus, MP3 files having a cutoff at 20Khz will have a much lower score than their WAV counterpart.

The threshold used to output the quality warning is arbitrary and based on my manual tests.
