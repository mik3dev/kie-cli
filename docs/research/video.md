# kie.ai video input modes (researched 2026-10-17 from docs.kie.ai/*.md OpenAPI specs)

Endpoint for ALL video models listed: `POST /api/v1/jobs/createTask`, body `{model, callBackUrl?, input:{...}}`; poll via unified Get Task Details (`/market/common/get-task-detail`). Exceptions: `POST /api/v1/omni/audio/create`, `POST /api/v1/omni/character/create` (Gemini Omni helpers, synchronous, return `data.audioId` / character id). Even Runway and Veo (old dedicated endpoints) are now createTask with `model` strings. Files must be URLs (File Upload API: base64 / stream / URL-upload exist). Some fields accept `oss://` (Kling, MiniMax) or `asset://{id}` (Seedance).

## 1. Taxonomy of input modes -> models (newest first)

| Mode | Models (model id) |
|---|---|
| Text-to-video | kling-3.0/video, kling-3.0-omni/text-to-video, kling/v3-turbo-text-to-video, kling-2.6/text-to-video, bytedance/seedance-2-5, seedance-2, -2-fast, -2-mini, bytedance/seedance-1.5-pro (no input_urls), wan/3-0-video(-prime), wan/2-7-text-to-video, wan/2-6-text-to-video, minimax-h3/text-to-video, happyhorse-1-1/text-to-video, happyhorse/text-to-video, pixverse-v6/text-to-video, grok-imagine/text-to-video, hailuo/02-text-to-video-*, veo-3-1 (TEXT_2_VIDEO), runway, gemini-omni-video, google/gemini-omni-flash-1-1 |
| Image-to-video (1 first frame) | kling-3.0/video, kling-3.0-omni/image-to-video, kling/v3-turbo-image-to-video, kling-2.6/image-to-video, seedance-2.x (first_frame_url), wan/2-7-image-to-video, wan/2-6(-flash)-image-to-video, minimax-h3/image-to-video, happyhorse(-1-1)/image-to-video, pixverse-v6/image-to-video, vidu-q4/image-to-video, grok-imagine/image-to-video (+grok image task_id), hailuo 2.3 pro/std, runway, veo-3-1 |
| First+last frame | seedance-2.x/2-5 (first_frame_url+last_frame_url), wan/3-0-video, wan/2-7-image-to-video, minimax-h3/image-to-video (either/both), pixverse-v6/transition (both required), kling-3.0/video (image_urls len 2), kling-3.0-omni/image-to-video (oneOf: 1 vs 2 imgs), kling/v2-1-pro + v2-5-turbo-i2v-pro (`tail_image_url`), hailuo/02-i2v-* (`end_image_url`), bytedance v1-lite-i2v (`end_image_url`), veo-3-1 (image_urls 2), gemini-omni (first_frame_url+last_frame_url) |
| Reference-to-video (multi-image) | kling-3.0-omni/reference-to-video, kling-3.0/video (kling_elements), seedance-2.x/2-5 (reference_image_urls), wan/3-0-video, wan/2-7-r2v, minimax-h3/reference-to-video, happyhorse(-1-1)/reference-to-video, pixverse-v6/reference-to-video, vidu-q4/reference-to-video, veo-3-1 (REFERENCE_2_VIDEO), grok-imagine (image_urls @image(n)), gemini-omni (image_urls + character_ids) |
| Reference video / video+audio refs | seedance-2.x/2-5, wan/3-0-video, minimax-h3 ref, kling-3.0-omni ref (video_urls), gemini-omni (video_list), vidu (audio only) |
| Video-to-video / edit / restyle | wan/2-7-videoedit, wan/2-6-video-to-video, wan/2-6-flash-video-to-video, happyhorse/video-edit, kling-3.0-omni/transformation, runway/gen4-aleph, seedance-2.5 (reference_video_urls + duration -1 matches input length), wan/2-2-animate-replace |
| Extend | grok-imagine/extend (task_id), runway/extend-ai-video (task_id), veo/extend (task_id), pixverse-v6/extend (taskId XOR video_url), wan/2-7-image-to-video (first_clip_url = continuation) |
| Upscale | topaz/video-upscale (video_url), grok-imagine/upscale (task_id), veo/get-1080p-video + veo/get-4k-video (task refs), bytedance/seedance-2-5-draft (draft_task_id -> 1080p) |
| Audio-driven avatar / lip-sync (image+audio) | kling/ai-avatar-standard, kling/ai-avatar-pro, infinitalk/from-audio, omnihuman-1-5 (+mask), wan/2-2-a14b-speech-to-video-turbo |
| Video+audio lip-sync | volcengine/video-to-video-lip-sync |
| Motion control (image + driving video) | kling-3.0/motion-control, kling-2.6/motion-control, wan/2-2-animate-move |
| Character/voice asset prep (two-step) | gemini-omni-character -> character_ids; gemini-omni-audio -> audio_ids; omnihuman-1-5/human-identification, /subject-detection (-> mask_url) |
| Draft -> final | seedance-2-5 (`draft:true`, 480p only) -> seedance-2-5-draft |
| Multi-shot | kling-3.0/video (multi_shots+multi_prompt), kling-3.0-omni/* (customize_multi_shots/prefer_multi_shots + multi_prompt), wan/2-6 (multi_shots bool), pixverse t2v/i2v (generate_multi_clip_switch) |
| File/link-to-video | wan/3-0-video (reference_file_urls, reference_link_urls) |
| Template effects | pixverse-v6/image-to-video (template_id enum, 1-2 images) |

## 2. Media field naming variants (inconsistent!)

| Concept | Names used (model) | Shape |
|---|---|---|
| Single input image | `image_url` (kling v2.1/2.5/avatar, hailuo, bytedance v1, wan 2.5/2.2, infinitalk, omnihuman, wan animate, vidu-q4 i2v) | string |
| | `image_urls` (kling 2.6/3.x, wan 2.6, happyhorse, pixverse i2v, grok, hailuo none) | array max 1 (or up to 7/2) |
| | `input_urls` (seedance-1.5-pro max2; kling motion-control image max1) | array |
| | `first_frame_url` (seedance 2.x, wan3, minimax-h3, wan2.7 i2v, gemini-omni) | string |
| | `first_frame_image_url` (pixverse transition) | string |
| | `first_frame` (wan 2.7 r2v) | string |
| | `first_clip_url` (wan 2.7 i2v video continuation) | string |
| Last frame | `last_frame_url`, `last_frame_image_url` (pixverse), `tail_image_url` (kling 2.1/2.5), `end_image_url` (hailuo, bytedance v1-lite), or index 1 of `image_urls` (kling 3.0, kling omni i2v, veo) | string / array |
| Ref images | `reference_image_urls` (seedance, wan3, minimax, vidu), `reference_image` ARRAY (happyhorse, wan2.7 r2v - singular name, array type; but wan2.7 videoedit `reference_image` is a STRING), `image_references[]` objects {image_url,type,ref_name} (pixverse), `image_urls` (kling omni ref, grok, gemini, veo), `elements[]`/`kling_elements[]` objects (kling) |
| Input video | `video_url` string (wan videoedit, happyhorse edit, wan animate, topaz, volcengine, runway aleph, pixverse extend, wan2.2 animate) / `video_urls` array max1 (kling omni, kling motion) / array max3 (wan 2.6 v2v) / `reference_video_urls` (seedance, wan3, minimax) / `reference_video` array (wan2.7 r2v) / `video_list[{url,start,ends}]` (gemini; "ends" not "end") / `element_input_urls` (kling element video) |
| Audio | `audio_url` string (kling avatar, infinitalk, omnihuman, wan speech, volcengine) / `driving_audio_url` (wan2.7 i2v) / `audio_url` (wan2.7 t2v custom audio) / `reference_audio_urls` array (seedance, wan3, minimax, vidu) / `reference_voice` string (wan2.7 r2v) / `element_input_audio_urls` (kling) / `audio_ids` (gemini, via audio/create) |
| Source-task ref | `task_id` (grok, runway, veo extend, veo 4k), `taskId` (pixverse extend, veo 1080p), `draft_task_id` (seedance draft) |
| Mask | `mask_url` ARRAY (omnihuman, singular name) |
| Audio on/off | `sound` (kling 2.6, kling 3.0), `audio` (kling omni, wan3, wan flash, vidu, gemini none), `generate_audio` (seedance), `generate_audio_switch` (pixverse); grok/minimax audio cannot be turned off |
| Aspect | `aspect_ratio` mostly; `ratio` (wan 2.7 t2v only) |
| Resolution | `resolution` (720p), `quality` (pixverse, runway), `mode` (kling 3.0 std/pro/4K; kling motion mode=720p/1080p!; hailuo none), `output_resolution` ("720"/"1080" no p, omnihuman), case: `768P`/`2K` (minimax), `480P/720P/1080P` (wan3), `512P/768P`, `768P/1080P` (hailuo) |
| Duration type | string enum (kling 2.6 "5"|"10", hailuo, wan 2.5/2.6, bytedance v1, gemini "4|6|8|10"), integer (seedance, wan3, wan2.7, minimax, happyhorse, pixverse, vidu, veo), number (grok t2v), string for grok i2v/kling v3-turbo/omni-transformation |
| Seed | `seed` mostly; `seeds` (veo extend, 10000-99999) |
| Safety | `nsfw_checker` (most), `enable_safety_checker` (bytedance v1) |

## 3. Per-family notes

**Kling**
- `kling-3.0/video`: one model, modes via fields. image_urls (1=first, 2=first+last; multi_shots true => first only), `sound`*, `duration`* string 3-15, `mode`* std|pro|4K, `multi_shots`*, `multi_prompt[{prompt<=500,duration 1-12}]` (docs say up to 5), `kling_elements[{name,description,element_input_urls (2-4 imgs JPG/PNG<=10MB OR 1 video MP4/MOV 3-8s), element_input_audio_urls (5-30s)}]` max 3, referenced `@name` in prompt. aspect_ratio 16:9|9:16|1:1 (required for pure t2v). prompt<=2500.
- Omni (kling-3.0-omni/{text,image,reference}-to-video, /transformation): `input` is oneOf-branchy. i2v: image_urls exactly 1 or exactly 2 (first/last). duration int 3-15, resolution 720p|1080p|4k, `audio` bool, customize_multi_shots / prefer_multi_shots (mutually exclusive), multi_prompt 1-6 shots, elements[] (max 3, 2-4 imgs or 1 video + audio). aspect_ratio `auto` only when no multi-shot. Images JPG/PNG<=50MB, >=300px, ratio 0.4-2.5.
- reference-to-video branches: (a) images only (image_urls<=7, aspect 16:9|9:16|1:1, audio ok), (b) video_urls (exactly 1, MP4/MOV<=200MB, 3-15.5s) only: aspect `auto`, audio MUST be false, (c) image_urls<=4 + video. Slot-budget rules between images/elements/video.
- transformation = video edit: video_urls(1)+optional image_urls(<=4)+elements; duration is STRING; audio must be false; aspect `auto` only for video-only.
- v3-turbo t2v/i2v: simple; prompt<=2500, duration string 3-15, resolution* 720p|1080p, i2v image_urls max1 (10MB).
- 2.6 t2v/i2v: sound* bool, duration* "5"|"10"; i2v image_urls max1.
- Motion control: 2.6 requires `character_orientation`* image|video, `mode`* 720p|1080p; input_urls(1 image, head/shoulders/torso) + video_urls(1, 3-30s; orientation=image => max 10s). 3.0 version: mode/character_orientation optional (video default), 3.0 model id `kling-3.0/motion-control`.
- Avatar: `kling/ai-avatar-standard|pro`: image_url + audio_url + prompt* (all required), image<=10MB, audio<=100MB.
- v2.1/v2.5 turbo (legacy): `image_url`, `tail_image_url` only on kling/v2-1-pro and v2-5-turbo-image-to-video-pro; model ids use `kling/v2-1-...` (no dash+dot like newer `kling-2.6/...`, `kling-3.0/...`, `kling/v3-turbo-...`).

**Bytedance / Seedance**: 2.5 (`bytedance/seedance-2-5`): prompt<=20480, first/last_frame_url (last needs first), reference_image_urls<=30 (jpeg/png/webp, <30MB), reference_video_urls<=10 (mp4/mov/mkv, 480p/720p), reference_audio_urls<=10 (wav/mp3/aac/ogg, 2-30s, <=15MB). Three MUTUALLY EXCLUSIVE scenarios: first-frame i2v, first+last, multimodal reference. duration int 4-30 or -1(auto; for edit tasks matches input), res 480p|720p|1080p, aspect incl `adaptive`, output_format mp4|mov, return_last_frame, generate_audio, web_search, `draft` (480p only) -> follow-up `seedance-2-5-draft` with `draft_task_id` (valid 7 days) at 1080p. 2.0 / fast / mini: same field names but ref limits 9 img / 3 video (2-15s, total<=15s) / 3 audio (total<=15s); duration 4-15 or -1; 2.0 res up to 4k (H.265), fast/mini max 720p; web_search t2v only; mini has no return_last_frame. Image refs accept `asset://{assetId}` (asset creation API not found in video index). 1.5-pro: `input_urls` 0-2 imgs (0 => t2v), aspect*, duration* 4-12, `fixed_lens`. V1 pro/lite: `image_url`, lite i2v `end_image_url`, duration "5"|"10".

**Hailuo (MiniMax)**: Legacy hailuo/02 & 2.3: i2v only `image_url` (+`end_image_url` on 02 i2v); no reference/audio. Newest = **minimax-h3/** {text,image,reference}-to-video: dur 4-15 int, res 768P|2K, t2v aspect required (no adaptive); i2v `first_frame_url`/`last_frame_url` (anyOf: at least one), ref `reference_image_urls`<=9, `reference_video_urls`<=3 (each 2-15s, total<=15s), `reference_audio_urls`<=3 (cannot be alone); need >=1 image or video. Audio always generated. Ref video seconds billed on top.

**Wan**: 3.0 (`wan/3-0-video`, `wan/3-0-video-prime`): one unified endpoint: t2v / first(+last) frame / all-purpose reference (images<=10 as Image1.., videos<=5, audios<=5; ≤15s totals) / file-to-video (reference_file_urls 1 doc/pdf/etc) / link-to-video (reference_link_urls 1). duration 2-30 or -1; res 480P|720P|1080P (uppercase P); model enum has NO default. 2.7: t2v (`ratio`!, `audio_url`), i2v (first_frame_url, last_frame_url, first_clip_url, driving_audio_url), r2v (`reference_image[]`, `reference_video[]`, `first_frame`, `reference_voice`; img+vid<=5; dur 2-10), videoedit (`video_url`* 2-10s ≤100MB, `reference_image` STRING, `audio_setting` auto|origin, duration 0=full). 2.6 / 2.6-flash: duration string 5|10|15, `multi_shots`, flash adds `audio`; v2v `video_urls` max 3. 2.5: `image_url`, prompt<=800. 2.2 a14b turbo (480/720p), speech-to-video (image+audio+frames params), animate-move/replace (video_url+image_url, res 480|580|720p).

**Grok Imagine**: t2v (`mode` fun|normal|spicy, dur 6-30, aspect 2:3|3:2|1:1|16:9|9:16), i2v (`image_urls`<=7 with `@image(n)` in prompt, OR `task_id`+`index` 0/1 from grok text-to-image; mutually exclusive; spicy not for external images; aspect only for multi-image), **1.5-preview** (`grok-imagine-video-1-5-preview`, unified, image_urls<=7, dur 1-15, res 480p|720p|1080p, no mode), extend (`task_id`,`prompt`,`extend_at`>=2 s,`extend_times` "6"|"10"; source MUST be a kie grok task), upscale (`task_id`, 720p|1080p). Audio always on.

**PixVerse V6** (`pixverse-v6/...`): t2v, i2v (image_urls<=2; with `template_id` effect enum -> duration forbidden), transition (first_frame_image_url*, last_frame_image_url*), extend (input is anyOf: `taskId` XOR `video_url`; seed* required), reference-to-video (`image_references[{image_url,type subject|background,ref_name<=30}]` 1-7, `@ref_name` in prompt). `quality`* 360p-1080p, duration 1-15, `generate_audio_switch`, `generate_multi_clip_switch`.

**HappyHorse**: happyhorse/ and happyhorse-1-1/ : t2v, i2v (image_urls max1), reference-to-video (`reference_image` 1-9 array; prompt uses "[Image 1]" / character1), video-edit (only v1: video_url 3-60s ≤100MB, reference_image 0-5, audio_setting). 1.1 adds more aspect ratios (4:5,5:4,9:21,21:9) and no seed/`audio_setting`; dur 3-15.

**Vidu Q4**: i2v (`image_url`), reference-to-video (`reference_image_urls` 1-15, `reference_audio_urls`<=3 MP3 only 3-12s), res 540p-4k, dur 3-16, `audio`.

**Gemini Omni**: `gemini-omni-video` and `google/gemini-omni-flash-1-1` (adds first/last frame, 360p res). Inputs: `image_urls`<=7, `video_list` (max 1 clip, {url,start,ends}, <=100MB, <=30s, ends-start<=10s; costs 2 image slots), `character_ids`<=3, `audio_ids`<=3. Slot budget: images + 2*videos + characters <= 7. Prereq calls: `/api/v1/omni/audio/create` (audio_id enum preset voice, name, voice_description, example_dialogue) and `/api/v1/omni/character/create` (descriptions*, image_urls 1-2 [portrait, optional body], audio_ids, character_name). first_frame_url mutually exclusive with image_urls/audio_ids/video_list/character_ids. duration* "4|6|8|10" (ignored if video input), aspect 16:9|9:16 only.

**OmniHuman 1.5** (`omnihuman-1-5`): image_url*, audio_url* (<60s, ≤10MB), optional `mask_url[]` (<=5, from `omnihuman-1-5/subject-detection`), prompt<=300, output_resolution "720"|"1080", pe_fast_mode. Helper tasks via createTask too: `omnihuman-1-5/human-identification` (image_url, <5MB), `/subject-detection`.

**Volcengine** `volcengine/video-to-video-lip-sync`: mode* lite|basic, video_url* (≤500MB, 360-1080p), audio_url* (≤10MB, 1-240s lite / 210 basic), separate_vocal, open_scenedet (basic only), align_audio / align_audio_reverse / templ_start_seconds (lite only).

**Topaz** `topaz/video-upscale`: video_url* (≤50MB), upscale_factor "1"|"2"|"4" (string).

**Infinitalk** `infinitalk/from-audio`: image_url*, audio_url*, prompt*, res 480p|720p, seed 10000-1000000.

**Runway**: `runway` (prompt, image_url optional => i2v else aspect_ratio required, duration 5|10 [10s not with 1080p], quality 720p|1080p, watermark text), `runway/extend-ai-video` (task_id, prompt, quality), `runway/gen4-aleph` (video_url*, prompt*, reference_image, aspect_ratio, seed, upload_cn). Model enums have no strict enum.

**Veo 3.1**: top-level model `veo-3-1`; real tier in `input.model` veo3_fast|veo3_lite|veo3; `generation_type` TEXT_2_VIDEO | FIRST_AND_LAST_FRAMES_2_VIDEO (1 or 2 image_urls) | REFERENCE_2_VIDEO (1-3 imgs, 8s only, fast/lite only); duration 4|6|8; res 720p|1080p|4k at generation; `veo/extend` (task_id, prompt, seeds, model fast|quality|lite); `veo/get-1080p-video` (`taskId`,`index`), `veo/get-4k-video` (`task_id`,`index` default 0).

## 4. Surprises / gotchas for the Go CLI

1. No uniform media schema: need per-model field-mapping layer (see table 2). Keep canonical CLI flags (--image, --first-frame, --last-frame, --ref-image, --video, --ref-video, --audio, --ref-audio) and map per model id.
2. Same concept different type: single `image_url` string vs `image_urls` array(max1) vs `first_frame_url`; `reference_image` is an ARRAY in happyhorse/wan2.7-r2v but STRING in wan2.7-videoedit; `mask_url` is array.
3. Typing traps: duration is string for kling 2.6/hailuo/wan2.5-2.6/bytedance-v1/gemini, integer for others, `number` for grok t2v; upscale_factor string; output_resolution string "720"; resolution casing varies (480P vs 480p; 768P/2K; 1080P).
4. `mode` means different things (Grok style fun/spicy; Kling 3.0 std/pro/4K; Kling motion resolution; Volcengine lite/basic). `quality` = resolution for PixVerse/Runway.
5. Extend/upscale on Grok, Runway, Veo, PixVerse(taskId), Seedance-draft reference a PRIOR kie task id, not a video URL (PixVerse extend alone also accepts video_url). Need task-ID chaining + inconsistent key (`task_id` / `taskId` / `draft_task_id`; veo 1080p=taskId, 4k=task_id).
6. oneOf / anyOf validation rules (client should validate before send): Kling omni (image count 1 vs 2, video forces audio=false & aspect=auto), MiniMax-H3 (first OR last; ref needs image or video; audio alone invalid), PixVerse extend (taskId XOR video_url), Seedance (first/last vs reference mutually exclusive; last needs first), Gemini Omni (first_frame excludes others; slot budget), Grok i2v (image_urls XOR task_id), Wan3 (frames vs references vs file vs link exclusive), PixVerse i2v (template_id forbids duration).
7. Fixed-length audio: Grok and MiniMax-H3 always generate audio; Kling omni with video input must have audio=false.
8. Billing surprises: MiniMax-H3 ref-video seconds billed, >5 ref images cost extra; Kling audio costs more; Veo 1080p/4k = 720p price + upgrade; Seedance 1.5 audio doubles price.
9. Prereq resources: Gemini Omni audio/character ids (synchronous non-createTask endpoints), OmniHuman subject-detection masks, Seedance `asset://` ids (creation endpoint not in video index; unclear), Kling element definitions inline.
10. Model id formats inconsistent: `kling-3.0/video`, `kling-3.0-omni/...`, `kling/v3-turbo-...`, `kling-2.6/...`, `kling/v2-1-pro`, `bytedance/seedance-2-5`, `bytedance/seedance-1.5-pro`, `grok-imagine-video-1-5-preview` (not slash), `omnihuman-1-5`, `gemini-omni-video`, `google/gemini-omni-flash-1-1`, `veo-3-1` (outer) + `input.model`.
11. Some pages omit `model` default/enum (wan3, runway), `input` not marked required (kling legacy, bytedance 1.5), and descriptions disagree with enums (kling 3.0 multi_prompt says up to 5 shots; omni says 6; 1:1 pro resolution 1440 vs 1080 in table vs mapping text).
12. Docs server flakily returns an HTML page instead of markdown for `.md`; retry.
13. Size limits are all over: images 10MB (kling legacy/hailuo/wan) / 20MB (grok, gemini, pixverse) / 30MB (seedance, minimax) / 50MB (kling omni, vidu); video 10MB (wan v2v/animate, per upload note) to 200-500MB; audio 10-100MB. Recommend only warning, let server validate.
14. Wan 2.7 videoedit/duration 0 = whole video; seedance-2.5/wan3 duration -1 = auto.
