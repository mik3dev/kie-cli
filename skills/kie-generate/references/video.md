# Video with kie-cli

Video costs the most. Before running: `kie-cli credits` and `kie-cli models price <model>`, then optionally `--dry-run`. Video models wait up to 30m by default; for several clips use `--no-wait` and `kie-cli task wait <taskId>`.

Field names for the same idea differ by model. Read `kie-cli models schema <model>` and use exactly what it lists. Durations are often strings (`duration: "5"`) and sometimes integers: the schema says which.

## Text to video

`kie-cli run kling/v2-5-turbo-text-to-video-pro -p prompt="a drone shot over a misty forest" -p duration=5 -p aspect_ratio=16:9 --out ./media`

Presets: `@video-calidad` (Kling 2.5 turbo, 5s), `@video-barato` (ByteDance lite, 480p, 5s). Pass `prompt`.

## Image to video (first frame)

- Single image as `image_url` (string): Kling 2.1/2.5 and ByteDance v1. Example:
  `kie-cli run kling/v2-5-turbo-image-to-video-pro -p prompt="the cat turns its head" -p image_url=./cat.png -p duration=5`
- Single image as `image_urls` (array of one): Kling 2.6/3.x, Wan 2.6, Grok.
- `first_frame_url` (string): Seedance 2.x, Wan 3, MiniMax H3.
- Preset: `@video-imagen` needs `prompt` and `image_url`.

## First and last frame

- Kling 2.1/2.5 image-to-video: `image_url` (first) plus `tail_image_url` (last).
- Veo 3.1: `image_urls` with two items [first, last] and `generation_type=FIRST_AND_LAST_FRAMES_2_VIDEO`. The tier is the inner `model` field (`veo3_fast`, `veo3_lite`, `veo3`). Example:
  `kie-cli run veo-3-1 -p prompt="a door opens" -p generation_type=FIRST_AND_LAST_FRAMES_2_VIDEO -p model=veo3_fast -p image_urls=./first.png -p image_urls=./last.png`
- Kling 3.0 and Kling omni: `image_urls` with two items [first, last].
- ByteDance v1 lite uses `end_image_url`; Seedance uses `last_frame_url` (needs `first_frame_url`).
- Preset: `@video-inicio-fin` (Veo 3.1 fast); pass `prompt` and `image_urls` as [first, last]. Check `kie-cli models price veo-3-1` first.

`generation_type` values for Veo: `TEXT_2_VIDEO`, `FIRST_AND_LAST_FRAMES_2_VIDEO`, `REFERENCE_2_VIDEO` (1 to 3 reference images, 8s, fast/lite tiers).

## Extend and chain

Extend, upscale and similar follow-ups reference a previous kie task, not a file. Take the id from the previous output's `ids`:

- `ids.task_id` is the finished task's id. Feed it to the follow-up model's id field, whose name varies (`task_id` for `veo/extend`, Grok and Runway extend; `taskId` or `video_url` for PixVerse extend; `draft_task_id` for Seedance draft). Example:
  `kie-cli run veo/extend -p task_id=<ids.task_id> -p prompt="the camera keeps moving forward"`
- `ids.last_frame_url` (present when the result carries a last frame, e.g. Seedance with `return_last_frame`) can be the next clip's first frame: `-p image_url=<ids.last_frame_url>`, or `first_frame_url` where that is the field.
- `ids.character_ids` lists character ids from character-creation helpers; pass them to the field the video model names (for example `character_ids`).

If an extend source must be a kie task (Grok, Runway, Veo), a plain video URL will not work; use the task id.

## Other modes (check the schema)

- Video edit/restyle: `video_url` or `video_urls` plus a prompt (Wan, Kling omni transformation, Runway Aleph).
- Lip-sync / avatars: image plus `audio_url` (Kling AI avatar, Infinitalk).
- Upscale: `topaz/video-upscale` (`video_url`, `upscale_factor` string).
- Audio on/off flag differs: `sound`, `audio` or `generate_audio`. Some models always render audio.

## Tips

- Pick the cheapest model that fits for drafts, then re-run the final with the better one.
- Some schemas have cross-field rules (mutually exclusive fields); `models schema` shows them as `variants` or `notes`. A `validation` error names the problem fields.
