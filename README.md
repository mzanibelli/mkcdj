# MkCDJ

A tool to manage an audio playlist with BPM analysis.

## Dependencies

You need to have `ffmpeg(1)`, and [bpm-tools](https://www.pogo.org.uk/~mark/bpm-tools) installed.

Ultimately, native implementations or low-level bindings would replace shell commands.

## Usage

- Run `mkcdj analyze PRESET PATH` to add a track to the collection
- Run `mkcdj compile PATH` to export all files to the given directory
- Run `mkcdj list` to preview the tracklist
- Run `mkcdj files` to print absolute file paths (for scripting)
- Run `mkcdj prune` to remove lost files from the current playlist

Add the `-v` flag to any of these commands get verbose output.

## Configuration

The `MKCDJ_STORE` environment variable contains the path to the current collection (a JSON file).

If unset, `/tmp/mkcdj.json` is used.

## Presets

A preset is a shorthand to hint the BPM detection. Each preset limits the detection to its predefined BPM range.
For example, the `dnb` (Drum & Bass) preset limits the detection from 165 to 180 BPM.

[Check the source to see the supported presets](https://github.com/mzanibelli/mkcdj/blob/master/mkcdj.go) or use `default` to get a wide range (1-200).

You can also pass a BPM value instead of a named preset. In that case the system will lookup the corresponding range.

## Export format

All files are exported in WAV 16 bits 44100Hz.

Additionally, waveform and spectrogram pictures of each file are generated in separate directories.
