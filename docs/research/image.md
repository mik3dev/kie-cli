# kie.ai IMAGE models: input-mode catalog (fetched from docs.kie.ai/*.md, 2026-10-17)

## 0. Common Market contract (all Market models)
- POST `https://api.kie.ai/api/v1/jobs/createTask`, body `{model, callBackUrl?, input:{...}}`; resp `{code,msg,data:{taskId}}` (HTTP 200 even on error; check `code`: 401/402/422/429/455/500).
- Poll: GET `/api/v1/jobs/recordInfo?taskId=` -> `data.state` waiting|queuing|generating|success|fail; `resultJson` = JSON STRING: `{"resultUrls":[...]}`; text-ish results `{"resultObject":{...}}`; `failCode`/`failMsg`.
- Inputs are URLs only (upload via File Upload API first; never raw bytes). Types jpeg/png/webp. Max 10MB (most) / 30MB (Seedream, nano-banana-2/pro/2.1, layer-decomp).
- Generated files expire (14d for 4o/Kontext).
- Params shape differs per model (no unified schema) -> CLI needs per-model registry.

## 1. Taxonomy of input modes -> models
(model id | endpoint is always createTask unless noted)

### T2I (prompt only)
seedream: `bytedance/seedream`(v3), `bytedance/seedream-v4-text-to-image`, `seedream/4.5-text-to-image`, `seedream/5-lite-text-to-image`, `seedream/5-pro-text-to-image`, `seedream/5-flash-text-to-image` | google: `google/imagen4`, `google/imagen4-fast`, `google/imagen4-ultra`, `google/nano-banana` | `flux-2/pro-text-to-image`, `flux-2/flex-text-to-image` | `grok-imagine/text-to-image`, `grok-imagine-image-2-0/text-to-image` | `gpt-image/1.5-text-to-image`, `gpt-image-2-text-to-image`, `gpt-image-2-5-flare-text-to-image`, `gpt-image-2-5-sunburst-text-to-image` | `z-image` | `qwen/text-to-image`, `qwen2/text-to-image`, `qwen2-1/text-to-image`, `qwen3/text-to-image`, `qwen3/pro-text-to-image` | `ideogram/v3-text-to-image` | `4o-image-api`, `flux1-kontext` (legacy-style, now also on createTask).
Hybrid (T2I when media omitted): `nano-banana-2`, `nano-banana-2-1`, `nano-banana-pro`, `nano-banana-2-lite`, `wan/2-7-image`, `wan/2-7-image-pro`, `gpt-image-2*-image-to-image` (input_urls optional!), 4o (`filesUrl` optional), Kontext.

### I2I / edit with 1 image (single string field)
`qwen/image-to-image` (image_url, strength 0-1), `qwen/image-edit` (image_url), `ideogram/v3-remix` (image_url, strength), `flux1-kontext` (input_image).

### Multi-reference / edit with N images (array field)
- image_urls[]: `bytedance/seedream-v4-edit`(<=10), `seedream/4.5-edit`(<=14), `seedream/5-lite-image-to-image`(<=14), `seedream/5-pro-image-to-image`(<=10), `seedream/5-flash-image-to-image`(<=10), `google/nano-banana-edit`(<=10), `nano-banana-2-lite`(<=10, optional), `grok-imagine/image-to-image`(<=5, prompt refs `@image1`), `grok-imagine-image-2-0/image-edit`(1-5), `qwen2-1/image-to-image`(1-10), `qwen3/image-to-image` & `qwen3/pro-image-to-image`(1-3)
- image_input[]: `nano-banana-2`, `nano-banana-2-1` (<=10), `nano-banana-pro` (<=8)
- input_urls[]: `flux-2/pro-image-to-image`, `flux-2/flex-image-to-image` (1-8, required), `gpt-image/1.5-image-to-image` (<=16, required), `gpt-image-2-image-to-image`, `gpt-image-2-5-flare-image-to-image`, `gpt-image-2-5-sunburst-image-to-image` (<=16, optional), `wan/2-7-image(-pro)` (<=9, optional)
- image_url as ARRAY (name lies): `qwen2/image-edit` (<=3)
- files_url[] (legacy 4o, <=5)

### Mask / inpaint edit
- `ideogram/v3-edit`: image_url + mask_url (same dims; both required)
- `ideogram/character-edit`: image_url + mask_url + reference_image_urls[<=1] (character ref)
- `qwen2-1/image-to-image`: optional `mask_url` (white=change, black=keep; needs exactly 1 image; ignores aspect_ratio/enhance_prompt; incompatible with background=transparent)
- `wan/2-7-image(-pro)`: `bbox_list` [[x1,y1,x2,y2]...] per input image (max 2 boxes/image) = box-region edit

### Segment-based edit (2-step, task chaining)
1. `grok-imagine-image-2-0/segment-map` (FREE): `input` is oneOf {task_id} | {image_url}. Result `resultObject:{segments_count, segments:[{maskUrl,name,index}]}`.
2. `grok-imagine-image-2-0/segment-edit`: `{prompt<=65000, task_id (from T2I task OR segment-map task), mask_indexs:[int,...] (0-based)}`. NOTE: no image field; uses task_id.

### Character consistency / remix / style ref
- `ideogram/character` (T2I w/ reference_image_urls[<=1], required), `ideogram/character-remix` (image_url + reference_image_urls[1] + image_urls[<=5 style refs] + reference_mask_urls (string!) + strength 0.1-1), `ideogram/character-edit` (above), `ideogram/v3-remix` (image_url + strength 0.01-1).
- Remix-like also: seedream edits, nano-banana multi-ref, gpt-image multi-ref.

### Upscale
`topaz/image-upscale` {image_url, upscale_factor "1"|"2"|"4" (string enum)}; `recraft/crisp-upscale` {image} (no params). (Grok `market/grok-imagine/upscale` is video; ignore.)

### Background removal
`recraft/remove-background` {image} - PNG/JPG/WEBP, max 5MB, <=16MP, 256-4096px.
Transparent-bg generation: gpt-image-2* `background:transparent|opaque|auto` (1K only); qwen2-1 `background` (png/webp only).

### Layer decomposition
`seedream/5-pro-layer-decomposition` {image_url (string, exactly 1, PNG/JPEG, <=30MB, 262,144-36M px, ratio 1:16-16:1), prompt? (<=5000; supports `<bbox>x1 y1 x2 y2</bbox>` normalized 0-1000), size* auto|1K|1.5K|2K, output_format png|jpeg(def jpeg; base only, layers always PNG)}; `seedream/5-flash-layer-decomposition` same but size optional (default auto), prompt minLength 4. Result: `resultObject.layers_data:[{z_index,size,output_format,url, bounding_box:{absolute,normalized}, name}]` (z_index 0 = base). Billed per image (base + each layer).

## 2. Media-field naming variants
| field | type | models |
|---|---|---|
| image_urls | string[] | seedream *-edit/i2i (v4 10, 4.5 14, 5-lite 14, 5-pro 10, 5-flash 10), nano-banana-edit(10), nano-banana-2-lite(10,opt, default []), grok-imagine/i2i(5), grok-2.0 image-edit(1-5), qwen2-1 i2i(1-10), qwen3 i2i & pro(1-3), ideogram char-remix (style refs, 5) |
| image_input | string[] | nano-banana-2, -2-1 (10); nano-banana-pro (8) |
| input_urls | string[] | flux-2 (8 req), gpt-image 1.5 (16 req), gpt-image-2/2.5 (16 opt), wan 2.7 (9 opt) |
| image_url | string | qwen/image-to-image, qwen/image-edit, ideogram v3-edit/v3-remix/char-*, topaz, seedream layer-decomp |
| image_url | string[] (!) | qwen2/image-edit (<=3) |
| image | string | recraft crisp-upscale, remove-background |
| reference_image_urls | string[<=1] | ideogram/character* (required) |
| mask_url | string | ideogram v3-edit/char-edit, qwen2-1 i2i |
| reference_mask_urls | string (not array) | ideogram/character-remix |
| mask_indexs (sic) | int[] | grok segment-edit (+ task_id) |
| task_id | string | grok segment-map / segment-edit (task chaining) |
| bbox_list | int[][] | wan 2.7 |
| input_image | string | legacy-style flux1-kontext |
| files_url (+deprecated file_url) | string[] / string | 4o |
| (legacy raw) filesUrl/fileUrl/inputImage | camelCase | old endpoints only |
Rule for CLI: unified `--image` (repeatable) mapped per-model to {kind: array|string|scalar-array-misnamed, field, min, max}.

## 3. Key params per model family (aspect/res/count/format)
| model | aspect/size field + enum | resolution/quality | count | out fmt | other |
|---|---|---|---|---|---|
| seedream v3 `bytedance/seedream` | image_size: square,square_hd,portrait_4_3,portrait_16_9,landscape_4_3,landscape_16_9 | - | - | - | guidance_scale 1-10, seed |
| seedream v4 t2i/edit | image_size: square,square_hd,portrait_4_3,portrait_3_2,portrait_16_9,landscape_4_3,landscape_3_2,landscape_16_9,landscape_21_9 (def square_hd) | image_resolution 1K/2K/4K | max_images 1-6 (set count in prompt too) | - | seed<=1e6, nsfw_checker; prompt<=5000 |
| seedream 4.5 | aspect_ratio* 1:1,4:3,3:4,16:9,9:16,2:3,3:2,21:9 | quality* basic(2K)/high(4K) | - | - | prompt<=3000 |
| seedream 5-lite | same ratios | quality* basic 2K/high 3K/ultra 4K | - | png/jpeg | prompt 4-2995 |
| seedream 5-pro | same ratios | quality* basic 1K/high 2K | - | png/jpeg | prompt 4-5000; first input image free, extra paid |
| seedream 5-flash | same ratios | size 1K/1.5K/2K (def 1K) | - | png/jpeg | |
| imagen4 / fast / ultra | aspect_ratio* 1:1,16:9,9:16,3:4,4:3,auto | - | - | - | negative_prompt, seed (string<=500 on imagen4/ultra; integer on fast) |
| nano-banana / -edit | aspect_ratio 1:1,9:16,16:9,3:4,4:3,3:2,2:3,5:4,4:5,21:9,auto; image_size = DEPRECATED alias | - | - | png/jpeg | prompt<=20000 |
| nano-banana-2 / 2-1 | aspect_ratio (adds 1:4,4:1,1:8,8:1) default auto | resolution 1K/2K/4K | - | png/**jpg** | |
| nano-banana-pro | aspect_ratio (no 1:4/1:8) | resolution 1K/2K/4K | - | png/jpg | image_input<=8 |
| nano-banana-2-lite | aspect_ratio* (incl 1:4..8:1,auto) | - | - | - | image_urls optional |
| flux-2 pro/flex | aspect_ratio* 1:1,4:3,3:4,16:9,9:16,3:2,2:3 (+auto for i2i = match 1st image) | resolution* 1K/2K | - | - | prompt 3-5000 |
| grok-imagine t2i | aspect_ratio* 2:3,3:2,1:1,16:9,9:16 | - | - | - | enable_pro (quality mode), nsfw_checker |
| grok-imagine/i2i | none | - | - | - | image_urls 1-5, `@image1` refs |
| grok-2.0 t2i / image-edit | aspect_ratio* 1:1,2:3,3:2,16:9,9:16 (+auto on edit) | - | - | - | prompt<=8000 (segment-edit 65000) |
| gpt-image 1.5 | aspect_ratio* 1:1,2:3,3:2 | quality* medium/high | - | - | prompt unbounded in schema |
| gpt-image-2 | aspect_ratio auto,1:1,3:2,2:3,4:3,3:4,5:4,4:5,16:9,9:16,2:1,1:2,3:1,1:3,21:9,9:21 | resolution 1K/2K/4K (2K excludes 5:4,4:5,3:1,1:3,9:21; 4K excludes 3:1,1:3,9:21) | - | - | background transparent/opaque/auto (1K only) |
| gpt-image-2-5 flare/sunburst | auto,1:1,3:2,2:3,4:3,3:4,16:9,9:16,21:9,27:16,16:27,9:8,8:9 (last four 1K only) | 1K/2K/4K | - | - | background 1K only |
| z-image | aspect_ratio* 1:1,4:3,3:4,16:9,9:16 | - | - | - | prompt<=1000 |
| qwen / qwen i2i / edit | image_size preset (square..landscape_16_9) | - | num_images "1".."4" (edit only) | png/jpeg | strength, acceleration none/regular/high, steps, guidance_scale, negative_prompt<=500, enable_safety_checker |
| qwen2 t2i | image_size ratio 1:1,3:4,4:3,9:16,16:9 | - | - | jpeg/png | seed |
| qwen2 edit | image_size ratio (adds 2:3,3:2,21:9) | - | - | jpeg/png | prompt<=800 |
| qwen2-1 | aspect_ratio (+auto for i2i) | resolution 1K/2K (2K slow, up to ~3 min) | - | png/webp/jpeg | background, enhance_prompt, seed |
| qwen3 / qwen3-pro | image_size ratio 1:1,3:2,2:3,4:3,3:4,16:9,9:16,21:9 | resolution 1K/2K | - | png/jpeg | prompt_extend, negative_prompt<=5000, seed |
| ideogram v3 t2i/remix | image_size preset (6 values) | rendering_speed TURBO/BALANCED/QUALITY | num_images "1".."4" (string!) | - | style AUTO/GENERAL/REALISTIC/DESIGN, negative_prompt, expand_prompt, seed |
| ideogram character* | image_size (character, remix); none on edit | rendering_speed | num_images "1".."4" | - | style AUTO/REALISTIC/FICTION, expand_prompt |
| wan 2.7 image / pro | aspect_ratio (only when no image input): 1:1,16:9,4:3,21:9,3:4,9:16,8:1,1:8 | 1K/2K (pro adds 4K; 4K only T2I non-sequential) | n: 1-4 (def 4) or 1-12 w/ enable_sequential (def 12) | - | enable_sequential, thinking_mode (T2I only), color_palette[{hex,ratio "xx.xx%"}] 3-10, bbox_list, watermark, seed |
| topaz upscale | - | upscale_factor "1"/"2"/"4" | | | |

## 4. Legacy endpoints (distinct from createTask)
Docs pages `/4o-image-api/generate-4-o-image.md` and `/flux-kontext-api/generate-or-edit-image.md` are now the NEW version (createTask, snake_case, model `4o-image-api` / `flux1-kontext`, poll via `/market/common/get-task-detail`). True legacy ("old-model") docs at `/old-model/...`:

| | 4o Image (legacy) | Flux Kontext (legacy) |
|---|---|---|
| create | POST `/api/v1/gpt4o-image/generate` | POST `/api/v1/flux/kontext/generate` |
| poll | GET `/api/v1/gpt4o-image/record-info?taskId=` | GET `/api/v1/flux/kontext/record-info?taskId=` |
| body (flat camelCase, no `model`/`input`) | `prompt`, `filesUrl[]` (<=5), `size`* (1:1,3:2,2:3), `callBackUrl`, deprecated `fileUrl`. prompt OR filesUrl required | `prompt`*, `model` flux-kontext-pro\|flux-kontext-max, `inputImage` (edit), `aspectRatio` (21:9,16:9,4:3,1:1,3:4,9:16), `outputFormat` jpeg\|png, `enableTranslation`, `promptUpsampling`, `safetyTolerance` (gen 0-6, edit 0-2, def 2), `uploadCn`, `watermark`, `callBackUrl` |
| result | `data.response.resultUrls[]`; `successFlag` (1 ok), `status` GENERATING/SUCCESS/CREATE_TASK_FAILED/GENERATE_FAILED, `errorCode`, `errorMessage`, `progress`, `paramJson`, `completeTime` | `data.response.{originImageUrl,resultImageUrl}` (single URL, not array); `successFlag` 0 generating,1 success,2 create failed,3 generate failed; `errorCode` 400/500/501 |
| create resp | `{code,msg,data:{taskId}}` | same |
New-version (createTask) equivalents: 4o -> `{model:"4o-image-api", input:{prompt, files_url[], size, file_url(deprecated)}}`; Kontext -> `{model:"flux1-kontext", input:{prompt, model:"flux-kontext-pro|max", input_image, aspect_ratio, output_format, enable_translation, prompt_upsampling, safety_tolerance, upload_cn, watermark}}` (note nested `input.model` tier). Kontext modes: T2I = prompt(+aspect_ratio); Edit = prompt+input_image (aspect_ratio optional: crop/pad; omitted keeps original). Kontext prompts English only unless enable_translation.

## 5. Gotchas
- Required-ness is inconsistent: aspect_ratio/quality marked required (*) on seedream 4.5/5, imagen4, flux-2, grok, z-image, gpt-image-1.5; optional on nano-banana-2/pro, gpt-image-2.
- Enum value types: `num_images` (ideogram, qwen/image-edit) and `upscale_factor` (topaz) are STRINGS; `n` (wan) integer; `max_images` (seedream v4) number.
- Output format enums differ: png/jpeg vs png/**jpg** (nano-banana-2/2-1/pro) vs png/webp/jpeg (qwen2-1) vs jpeg-only default for layer-decomp.
- Resolution naming: `quality` (seedream 4.5/5-lite/5-pro, gpt-1.5 = true quality), `size` (5-flash, layer), `resolution` (most), `image_resolution` (seedream v4), `image_size` (preset OR ratio OR deprecated alias depending on model!). `image_size` means ratio for nano-banana(deprecated)/qwen2/qwen3 but preset names for seedream v4/ideogram/qwen.
- seedream 5 quality meaning shifts per tier: 5-pro basic=1K/high=2K; 5-lite basic=2K/high=3K/ultra=4K; 4.5 basic=2K/high=4K.
- imagen4 `seed`: string on imagen4/ultra, integer on fast.
- ideogram `reference_mask_urls` is a string; `reference_image_urls` is array capped at 1 (extra ignored). Mask must match image dimensions.
- Grok segment-edit takes `task_id` (no image upload); segment-map must run first for custom images (free); `mask_indexs` spelling.
- grok-imagine/i2i prompt references images as `@image1` (type `@image(n)` + space).
- Seedream layer-decomp: prompt `<bbox>` coords normalized 0-1000; Pro requires `size`, Flash doesn't; billed per layer, unpredictable count in auto.
- Seedream 5 i2i: first input image free, extras paid.
- gpt-image-2: `background` valid only at 1K; some ratios invalid at 2K/4K; 2.5 variants have odd 27:16 etc. 1K-only.
- wan 2.7: aspect_ratio ignored when images given; thinking_mode only T2I; 4K only pro+T2I+non-sequential; bbox_list outer length must equal input_urls.
- qwen2-1 mask: exactly 1 image; incompatible with transparent bg; 2K up to ~3min.
- flux-2 `aspect_ratio: auto` requires an input image (i2i only; T2I enum excludes auto).
- Prompt length limits vary wildly (qwen2/edit 800, z-image 1000, seedream 4.5 3000, 5-lite 2995, nano 20000, gpt 20000, grok-2 8000/65000, grok i2i 390000).
- Image size caps: 10MB default; 30MB seedream/nano-2/pro/2.1; recraft bg-removal 5MB/16MP/256-4096px; layer-decomp 30MB & pixel range; 4o legacy 26,214,400 bytes.
- HTTP 200 for application errors; always read body.code. docs.kie.ai rate-limits bursts (returns HTML) - throttle doc fetches.
- Nano Banana Pro llms.txt title "Pro Image to Image" is T2I+multi-ref in one endpoint (model id `nano-banana-pro`, no "google/" prefix); same for `nano-banana-2`, `nano-banana-2-1`, `nano-banana-2-lite`. Prefixes vary: `google/`, `bytedance/`, `seedream/`, bare.
- Images expire 14 days (legacy docs); download results promptly. No Imagen/Nano docs list multi-count; only seedream v4 (max_images), ideogram/qwen (num_images), wan (n) return multiple.
