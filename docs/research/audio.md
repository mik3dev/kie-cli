# kie.ai AUDIO capabilities (researched 2026-10-17 from docs.kie.ai/*.md)

KEY FINDING: Suno NO LONGER has its own endpoints in the current docs. ALL Suno ops now go through the unified Market API:
`POST https://api.kie.ai/api/v1/jobs/createTask` body `{model:"ai-music-api/<op>", callBackUrl?, input:{...snake_case...}}`, poll `GET /api/v1/jobs/recordInfo?taskId=`.
Legacy endpoints (`/api/v1/generate`, `/api/v1/generate/extend`, ...) still exist under docs.kie.ai/old-model/suno-api/* but are not in llms.txt; don't build on them.
Auth: `Authorization: Bearer <key>`. HTTP is always 200; check body `code` (200 ok, 401 key, 402 credits, 422/500 bad req, 429 rate, 451/455 etc).
Doc quirk: apidog schema "required" lists for Suno are bogus (everything listed); real rules are in prose below.
Generated files retained 14 days.

## 1. Capability taxonomy -> model / endpoint

| Capability | `model` (createTask) | Notes |
|---|---|---|
| Music gen (text/lyrics, +image/video/audio refs) | `ai-music-api/generate` | Suno V6 family |
| Extend (from prior audio_id) | `ai-music-api/extend` | needs `audio_id` |
| Upload+Cover (cover/restyle an audio) | `ai-music-api/upload-and-cover-audio` | `upload_url` OR (`task_id`+`audio_id`) |
| Upload+Extend | `ai-music-api/upload-and-extend-audio` | `upload_url`, `continue_at` |
| Add Instrumental (backing under a vocal upload) | `ai-music-api/add-instrumental` | `upload_url` |
| Add Vocals (vocals over instrumental upload) | `ai-music-api/add-vocals` | `upload_url` |
| Replace section (infill) | `ai-music-api/replace-section` | task_id+audio_id OR upload_url+model |
| Mashup (2 audios) | `ai-music-api/mashup` | `upload_url_list` (2) |
| Persona (style persona) | `ai-music-api/generate-persona` | task_id+audio_id; once per audio |
| Voice clone (Suno Voice) | `ai-music-api/validation-phrase` -> `ai-music-api/create-voice`; `.../regenerate-phrase`, `.../check-voice` | 3-step flow; all free |
| Lyrics gen | `ai-music-api/generate-lyrics` | prompt only |
| Timestamped lyrics | `ai-music-api/timeStamped-lyrics` | SYNC result |
| Boost style | `ai-music-api/boost-music-style` | SYNC-ish text result |
| Sounds / loops / SFX-music | `ai-music-api/sounds` | prompt<=500, loop/BPM/key |
| WAV conversion | `ai-music-api/convert-to-wav-format` | task_id+audio_id |
| Music video (MP4) | `ai-music-api/create-music-video` | task_id+audio_id |
| Cover IMAGE for a track | `ai-music-api/cover-generate` | task_id; ~2 images (not audio cover!) |
| Stem separation / vocal removal | `ai-music-api/separate-vocals` | types separate_vocal / split_stem / split_stem_advanced |
| MIDI from stems | `ai-music-api/generate-midi-from-audio` | needs prior separation task; free |
| Recover expired audio links | `ai-music-api/suno-recovery-audio` | special polling (see gotchas) |
| TTS single voice | `elevenlabs/text-to-speech-multilingual-v2`, `elevenlabs/text-to-speech-turbo-2-5` | |
| TTS dialogue | `elevenlabs/text-to-dialogue-v3` | |
| TTS multi-speaker/style/clone | `google/gemini-3-8-flash-tts`, `google/gemini-3-8-flash-lite-tts`, `google/gemini-3-1-flash-tts`, `google/gemini-2-5-pro-tts` | |
| Audio isolation (voice denoise) | `elevenlabs/audio-isolation` | **DISCONTINUED** |
| Voice preset for video (not audio output) | `POST /api/v1/omni/audio/create` (Gemini Omni Audio) | sync, free; returns audioId for video `audio_ids` |

## 2. Suno endpoint table (all `POST /api/v1/jobs/createTask`; `input` fields)

Common optional tuning (where listed): `negative_tags`(str), `vocal_gender`(m|f, probabilistic), `style_weight`/`weirdness_constraint`/`audio_weight`(0-1, 2dp), `variety`(int 0-4, def 1), `persona_id`, `persona_model`(style_persona|voice_persona), `lyrics`(<=5000, wins over `prompt`), `duration`(10-360s; V6 family only).
Models: enum V4,V4_5,V4_5PLUS,V4_5ALL,V5,V5_5,V6,V6_MINI,V6_WILD. V4..V5_5 are marked Discontinued/struck-through; docs say "Use V6, V6_MINI or V6_WILD" (V6=expressive vocals, V6_MINI=fast/light, V6_WILD=experimental). Limits (V6*): style<=1000, lyrics/prompt<=5000, title<=80 (gen/cover) or 100 (extend/upload-extend/mashup). Old: V4 prompt 3000/style 200.

| op (model suffix) | required input | notes |
|---|---|---|
| generate | `custom_mode`(bool), `instrumental`(bool), `model`(V*) | Custom: `title` req (<=80); need >=1 of style/lyrics/negative_tags; `prompt`=strict lyrics unless `lyrics` given; instrumental=true -> no vocals. Non-custom: prompt<=3000 is just an idea (lyrics auto), need >=1 of image_urls/video_urls/audio_urls/style/lyrics; `image_urls`(<=5,10MB each jpeg/png/webp/bmp), `video_urls`(1, <=100MB, <=241s mp4/mov/webm), `audio_urls`(6s-30min, <=500MB) ONLY in non-custom; custom-only: duration, vocal_gender, style_weight, weirdness_constraint, audio_weight, variety (don't send in non-custom). Attachments total (style+lyrics+image+video+audio) <=10 |
| extend | `audio_id`, `model` | optional continue_at (>0, <source dur), prompt/lyrics/style/title, instrumental (if true: no lyrics/prompt/vocal_gender). **Model must match source audio's model** |
| upload-and-cover-audio | `instrumental`, `model`, + (`upload_url` XOR `task_id`+`audio_id`) | source <=8 min; don't send image_urls; instrumental=true forbids lyrics/prompt/vocal_gender; title<=80 |
| upload-and-extend-audio | `upload_url`, `model` | always custom mode (don't send custom_mode); continue_at in (0, dur); source <=8 min; model must match |
| add-instrumental | `upload_url`, `title`, `negative_tags`(<=200), `tags`(<=1000) | model def V6; only V4_5PLUS,V5,V5_5,V6,V6_MINI,V6_WILD |
| add-vocals | `upload_url`, `title`, `negative_tags`(<=200), `style`(<=1000) | optional prompt/lyrics; same model subset |
| replace-section | `prompt`(new lyrics), `tags`, `title`, `infill_start_s`, `infill_end_s`, `full_lyrics` + (`task_id`+`audio_id` XOR `upload_url`+`model`) | end>start; >=10s; <=50% of track; 2dp; optional lyrics (segment), negative_tags, persona_* (def style_persona) |
| mashup | `upload_url_list`(2 public URLs; only first 2 used), `model`, `style`, `title` | duration def 20 |
| generate-persona | `task_id`,`audio_id`,`name`,`description` | optional vocal_start(0)/vocal_end(30), style. Task must be complete; once per audio_id; task from generate/extend. Returns persona_id |
| validation-phrase | `voice_url`, `vocal_start_s`, `vocal_end_s`, `language`(en,zh,es,fr,pt,de,ja,ko,hi,ru) | callback/status gives `validateInfo` phrase, status `wait_validating`; free |
| create-voice | `task_id`(of validation task), `verify_url`(user reading phrase, singing preferred) | optional voice_name, description, style, singer_skill_level(beginner*/intermediate/advanced/professional); returns instantly; free. **Voice ID = validation-phrase taskId** (also resultJson.resultUrls[0]); ready when that task state=success. Use as `persona_id` + `persona_model:"voice_persona"` |
| regenerate-phrase | `task_id` (validation task) | updates same task; keep polling original task_id; free |
| check-voice | `task_id` | SYNC `data.isAvailable`; no callBackUrl; free |
| generate-lyrics | `prompt` | ~2-3 variations |
| timeStamped-lyrics | `task_id`,`audio_id` | SYNC: `data.code`==200 && `data.data.{alignedWords[{word,success,startS,endS,palign}], waveformData[], hootCer, isStreamed}`; instrumental -> none. (callback form snake_case aligned_words/start_s/end_s/p_align) |
| boost-music-style | `content` | resp `data.{taskId,result(text),successFlag 0/1/2,creditsConsumed,...}` |
| sounds | `prompt`(<=500), `model`(V5,V5_5,V6,V6_WILD,V6_MINI) | sound_loop(bool), sound_tempo(1-300 BPM, omit=auto), sound_key(Any,Cm..B list), grab_lyrics(bool) |
| convert-to-wav-format | `task_id`,`audio_id` | poll: resultJson.resultUrls[0]; callback `data.audio_wav_url` |
| create-music-video | `task_id`,`audio_id` | optional author(<=50), domain_name(<=50); poll resultUrls[0]; callback `data.video_url` |
| cover-generate | `task_id` | cover art; callback `data.images[]` |
| separate-vocals | `task_id`+(`audio_id` XOR `audio_url`), `type` | type: separate_vocal(def, 2 stems)/split_stem(<=12 stems)/split_stem_advanced (needs `stem_name`, ~100-value enum e.g. 'Lead Vocal','Drum Kit','Bass','Piano','808'...; returns extract+remove groups). Billed per call, no cache; failed not charged |
| generate-midi-from-audio | `task_id` (completed separation) | optional `audio_id` = stem id from separation CALLBACK (`vocal_removal_info.origin_data[].id`); omitted -> all stems. Free. Docs say callBackUrl required |
| suno-recovery-audio | `task_id` | see gotchas |

### Polling / result shape
- Generic: `GET /api/v1/jobs/recordInfo?taskId=` -> `data.{taskId,model,state(waiting|queuing|generating|success|fail),param(JSON str; input is itself JSON str),resultJson(str),response(parsed obj),failCode,failMsg,costTime,completeTime,createTime,creditsConsumed}`. Backoff start 2-3s. 422 = bad/foreign taskId.
- Music tracks (generate/extend/upload-*/add-*/mashup/sounds): `data.response.data[]` each `{id (=audioId), audio_url, stream_audio_url, image_url, prompt, model_name, title, tags, createTime, duration}`. Usually 2 tracks per task.
- WAV/video: `resultJson.resultUrls[0]`. Separation: `resultJson.resultUrls` (filenames end `_Vocals.mp3` etc.). Lyrics: `resultJson.resultObject.lyricsData[] {text,title,status,error_message,tags}`. MIDI poll only has summary; notes via callback `data.instruments[].notes[] {pitch,start,end,velocity(0-1)}` or `GET /api/v1/midi/record-info?taskId=`.
- Callback (POST to callBackUrl, 15s timeout, return 200 fast; optional webhook signature, see /common-api/webhook-verification). Music envelope: `{code, msg, data:{callbackType: text|first|complete, task_id, data:[tracks]}}`. Stages: `text` (lyrics text ready), `first` (1st track done; stream_audio_url usable), `complete` (all). Docs: "only complete is guaranteed". Failure: code!=200 (e.g. 501), callbackType complete, `data.data` absent. Callback uses snake_case `task_id`; recordInfo uses `taskId`. Lyrics cb: `data.data[]{text,title,status,error_message}`, fail code 400 (moderation)/500. Separation cb: `data.vocal_removal_info{vocal_url,instrumental_url,origin_data[{id,audio_url,stem_type_group_name,duration}]}` (split_stem adds backing_vocals_url,bass_url,brass_url,drums_url,fx_url,guitar_url,keyboard_url,percussion_url,strings_url,synth_url,woodwinds_url). Voice-phrase cb: `data.{taskId,validateInfo,status}`; voice cb: `data.{taskId,status success|fail,errorCode,errorMessage}`.
- `callBackUrl`: OPTIONAL everywhere in createTask (except MIDI where doc says required, and check-voice has none). Polling is a valid alternative. For a CLI: poll, no callback.

## 3. Operations needing chaining (prior ids)
- audioId = `data.response.data[i].id` from a finished music task (taskId + audioId pair).
- extend: audio_id (+ same model). replace-section (existing), convert-to-wav, create-music-video, timeStamped-lyrics, generate-persona: `task_id` + `audio_id`. cover-generate: `task_id` only. separate-vocals: task_id (+audio_id optional) or `audio_url`. upload-and-cover: may use task_id+audio_id instead of upload_url.
- MIDI: separation task_id; stem `audio_id` ONLY obtainable from separation callback `origin_data[].id` (not via recordInfo) -> CLI must either use callback/tunnel or just omit audio_id (all stems).
- Persona: persona_id from generate-persona -> generate/extend/cover/mashup/replace-section `persona_id` (+persona_model style_persona).
- Voice clone chain: validation-phrase(taskId=T) -> user reads/records `validateInfo` -> create-voice(task_id=T, verify_url) -> poll T until success -> persona_id=T, persona_model=voice_persona. regenerate-phrase(task_id=T) if phrase bad.
- Needs hosted URL input (upload_url/audio_urls/voice_url/verify_url/audio_url): use kie File Upload API (/file-upload-api/quickstart) to get URL first.

## 4. TTS / ElevenLabs / Gemini details
All: `POST /api/v1/jobs/createTask`, poll recordInfo; output URL in `resultJson.resultUrls[0]`.
- `elevenlabs/text-to-speech-multilingual-v2` / `turbo-2-5`: input `text`(<=5000, req), `voice`(67-value enum of ElevenLabs voice IDs, def EkK5I93UQWFDigLMpZcX=James; multilingual-v2 lists voice required, turbo only text required; doc text mentions names like "Rachel" but enum is IDs `id - Name - desc`; preview `https://static.aiquickdraw.com/elevenlabs/voice/<voice_id>.mp3`), `stability`(0-1, .5), `similarity_boost`(0-1, .75), `style`(0-1, 0), `speed`(0.7-1.2, 1), `timestamps`(bool, word timestamps), `previous_text`/`next_text`(<=5000, continuity), `language_code`(ISO 639-1; ONLY turbo-2.5 enforces — multilingual-v2 errors if sent).
- `elevenlabs/text-to-dialogue-v3`: `dialogue[]{text, voice(ID enum)}` (sum of text <=5000), `stability` enum {0,0.5,1} def .5, `language_code` (74-code enum; omit=auto). Audio tags in text implied by v3 (not documented).
- Gemini TTS (`google/gemini-*-tts`): input `speakers[]{speaker_id:"Speaker N", voice_name ...}` + `dialogue_turns[]{speaker_id,text,(style)}` (both required), `temperature`(0-2, def 1). 1-2 speakers.
  - 3.8-flash & 3.8-flash-lite: output WAV 16-bit PCM 24kHz mono. 70 preset `voice_name` (default Fola; groups Tutor/Commercial/Instructional/Podcast/Storyteller/Assistant/Call Center/Concierge/Tech Support/Professional e.g. Zephyr,Kore,Puck,Fenrir,Fola,Cruz...). Per-turn free-text `style`. `filler_words`(bool, only w/ 2 speakers). `replica{source_audio_url, consent_audio_url}` voice clone (3.8 only; mutually exclusive with voice_name; WAV only, 24k mono 16-bit, <=10MB each, 10-30s ref, consent sentence "I am the owner of this voice and I consent to Google using this voice to create a synthetic voice model."; same adult/mic/room; AI audio rejected; not saved). Inline tags in text `<laugh> <gasp> <sigh> <whispers> <short pause> <long pause> ...` (40 tags).
  - 3.1-flash & 2.5-pro: 30 voices (Achernar..Zubenelgenubi incl Charon,Fenrir,Puck,Kore,Leda,Orus,Zephyr), plus `scene`, `sample_context` (strings), per-speaker `audio_profile`(free text), `accent` enum {Neutral, American (Gen), American (Valley), American (South), British (RP), British (Brixton), Transatlantic, Australian}, `style` enum {Vocal Smile, Newscaster, Whisper, Empathetic, Promo/Hype, Deadpan}, `pace` enum {Natural, Rapid Fire, The Drift, Staccato}. Bracket tags in text like `[shouting]`. No replica/filler_words/per-turn style.
- `elevenlabs/audio-isolation`: `audio_url`, mp3/wav/aac/mp4/ogg <=10MB. DISCONTINUED -> exclude from CLI (or mark deprecated). Alternative for vocal removal: Suno separate-vocals (Suno-generated or uploaded audio_url).
- Gemini Omni Audio (`POST /api/v1/omni/audio/create`, NOT createTask): body `{audio_id: <enum preset voice e.g. achernar,kore,puck,fenrir... 30 names>, name(<=210), voice_description(<=20000), example_dialogue(<=120)}` -> `{code:200,data:{audioId,name}}`; free; sync; no audio file produced. audioId goes into `audio_ids` of gemini-omni-video / google/gemini-omni-flash-1-1 / gemini-omni-character (video domain; out of scope for audio CLI except as helper).

## 5. Gotchas
- Suno request model names: top-level `model` = `ai-music-api/<op>`; `input.model` = Suno version (V6...). Don't confuse. replace-section top-level model enum only `ai-music-api/replace-section`.
- Different snake_case in input (`custom_mode`, `audio_id`, `upload_url`) vs older camelCase (customMode, audioId, uploadUrl) of legacy API.
- `callBackUrl` sample says optional; MIDI doc says required. Sync "tasks" (timestamped lyrics, check-voice, boost style, omni audio) return data inline; timestamped lyrics needs both outer code and `data.code` ==200.
- recovery-audio is inconsistent: createTask example uses `ai-music-api/suno-recovery-audio` input.task_id, but prose says `POST /api/v1/suno/recovery {"taskId"}` and poll `GET /api/v1/suno/recovery/record-info?task_id=` (NOT recordInfo): code 201 = running (data null), 200 = done (entry per track), 500 = all failed; 2s interval. Use for expired Suno `source_audio_url`.
- State machine for polling: waiting/queuing/generating/success/fail; also stream_audio_url available during `first` stage before final audio_url.
- Validate client-side: custom_mode rules (title required; at least one of style/lyrics/negative_tags), instrumental conflicts, duration only V6* + custom mode, continue_at < source duration, replace-section bounds (>=10s, <=50%), upload sources <=8 min, mashup exactly 2 URLs, split_stem_advanced requires stem_name, split-stem credits charged per call.
- `input` echoed in `param` is a JSON string within JSON string (parse twice).
- Voice ID == validation taskId; create-voice returns success before voice is ready.
- Persona: one per audio_id; persona_model voice_persona needs V5+/V6; vocal_gender only a hint.
- ElevenLabs enums are IDs only; ship a name->ID table (67 voices on each page; names in description text). language_code differs per model.
- Gemini 3.8 `voice_name` default is `Fola` if omitted; voice_name vs replica mutually exclusive; 3.1/2.5 enums differ from 3.8.
- Discontinued: elevenlabs/audio-isolation; Suno V4..V5_5 flagged discontinued (default CLI to V6).
- Failed-task billing: separation failed = free; MIDI/voice endpoints free.
