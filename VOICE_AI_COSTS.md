# Affordable Voice AI for Phone Calls

Pricing researched July 11, 2026. Vendor rates change frequently, so verify
the linked pricing pages before making a long-term commitment.

## Summary

Eight to ten cents per minute is a convenience or platform price, not the
underlying cost floor. A carefully selected stack can cost approximately
**$0.01–$0.04 per connected minute**, including US telephony.

The estimates below generally assume that the caller and AI each speak for
half of a connected minute. Actual costs depend on talk ratio, conversation
length, context handling, billing increments, chosen voices, and call
direction.

| Approach | Approximate cost | Best fit |
| --- | ---: | --- |
| Plivo managed AI agent | $0.0355/min inbound or $0.0415/min outbound | Quick, inexpensive managed deployment |
| OpenAI Realtime mini | About $0.015/min plus telephony | Natural speech-to-speech interaction |
| Gemini Flash Live | About $0.0115/min plus telephony, before context effects | Low-cost native audio with careful context management |
| Custom STT → LLM → TTS | About $0.01–$0.015/min including inbound telephony | Lowest predictable production cost |
| Self-hosted models | Telephony plus hosting | High volume and maximum operational control |

## Why Twilio appears expensive

Twilio's US pricing distinguishes between several products:

- Conversation Relay: **$0.07/minute**
- Media Streams: **$0.0044/minute**
- Conversational Intelligence streaming transcription: **$0.027/minute**
- US local inbound call: **$0.0085/minute**
- US local outbound call: **$0.014/minute**

This means a price around eight cents per minute is probably the bundled
Conversation Relay and telephony cost, not the raw cost of speech recognition.
Conversation Relay includes orchestration and speech services, for which
Twilio charges a substantial convenience premium.

Source: [Twilio US Voice pricing](https://www.twilio.com/en-us/voice/pricing/us)

## Option 1: Plivo's managed AI agent

Plivo lists its managed Voice AI Agent at **$0.03/minute**, with US local
calling priced separately:

- Inbound local calling: **$0.0055/minute**
- Outbound local calling: **$0.0115/minute**
- Audio streaming and noise cancellation: included

The resulting headline totals are approximately:

- Inbound: **$0.0355/minute**
- Outbound: **$0.0415/minute**

Listed platform features include realtime transcription, knowledge bases,
function and tool calling, call recording, call transfer, and multilingual
support. Plivo bills Voice and AI Agent calls in 60-second intervals. Confirm
whether a selected premium voice or model produces an additional charge.

Source: [Plivo pricing](https://www.plivo.com/pricing/)

## Option 2: OpenAI Realtime mini

The less expensive OpenAI speech-to-speech model is
`gpt-realtime-2.1-mini`, not the full Realtime model. Its audio pricing is:

- Audio input: **$10 per million tokens**
- Cached audio input: **$0.30 per million tokens**
- Audio output: **$20 per million tokens**

Using OpenAI's published audio-token-to-duration relationship, this is
approximately:

- Caller audio: **$0.006 per minute spoken**
- AI audio: **$0.024 per minute spoken**
- A 50/50 conversation: **$0.015 per connected minute**

Telephony, text tokens, context processing, and tool calls are additional.
The model supports SIP, WebSocket, and WebRTC connections, allowing a SIP
carrier to route calls to it without using Twilio Conversation Relay.

Sources:

- [OpenAI GPT-Realtime-2.1 mini pricing](https://developers.openai.com/api/docs/models/gpt-realtime-2.1-mini)
- [OpenAI's published audio cost conversion](https://openai.com/index/introducing-the-realtime-api/)

## Option 3: Gemini Flash Live

Google lists `gemini-3.1-flash-live-preview` at:

- Audio input: **$0.005 per minute**
- Audio output: **$0.018 per minute**
- A 50/50 conversation: approximately **$0.0115 per connected minute**

This excludes telephony. Gemini Live also reprocesses and re-bills tokens in
the accumulated session context on subsequent turns. Long calls can
therefore cost more than the simple per-minute estimate. Context-window
compression and bounded conversation history are important for predictable
costs.

Sources:

- [Gemini API pricing](https://ai.google.dev/gemini-api/docs/pricing)
- [Gemini Live API billing behavior](https://ai.google.dev/gemini-api/docs/live-api/best-practices)

## Option 4: A custom STT → LLM → TTS pipeline

A low-cost modular architecture would look like this:

```text
Plivo or Telnyx phone number
        ↓
Pipecat voice pipeline
        ↓
AssemblyAI streaming speech-to-text
        ↓
Small, fast text LLM
        ↓
Deepgram Aura, Amazon Polly, or Google text-to-speech
```

A representative inbound US cost calculation is:

| Component | Approximate connected-minute cost |
| --- | ---: |
| Plivo inbound phone call | $0.0055 |
| AssemblyAI streaming STT | $0.0025 |
| Small text LLM | Less than $0.001 in a typical concise conversation |
| Standard TTS at a 50% AI talk ratio | About $0.0015 |
| Neural TTS at a 50% AI talk ratio | About $0.0055–$0.006 |

This produces an approximate total of:

- **$0.01/minute** with standard TTS
- **$0.014–$0.015/minute** with natural neural TTS

These totals exclude application hosting. Outbound telephony costs about
$0.006/minute more than inbound telephony at Plivo's listed US rates.

Relevant prices and sources:

- AssemblyAI Universal-Streaming: **$0.15/hour**, or **$0.0025/minute**.
  [AssemblyAI streaming STT](https://www.assemblyai.com/products/streaming-speech-to-text)
- Deepgram Nova-3 streaming STT: promotional rate starting around
  **$0.0048/minute**; Flux English starts around **$0.0065/minute**.
  [Deepgram pricing](https://deepgram.com/pricing)
- Amazon Polly Standard: **$4 per million characters**; Neural:
  **$16 per million characters**.
  [Amazon Polly pricing](https://aws.amazon.com/polly/pricing/)
- Google Standard and WaveNet legacy voices: starting at **$4 per million
  characters**; Neural2: **$16 per million characters**.
  [Google Cloud Text-to-Speech pricing](https://cloud.google.com/text-to-speech/pricing/)

### Orchestration

[Pipecat](https://github.com/pipecat-ai/pipecat) is an open-source realtime
voice framework. It supports Plivo, Telnyx, Twilio, AssemblyAI, Deepgram,
OpenAI, Gemini, Polly, Piper, Ollama, and other hosted or local services. A
self-hosted Pipecat deployment avoids an additional per-minute orchestration
fee, although it introduces hosting and operational work.

## Very inexpensive turn-based transcription

Groq offers Whisper Large V3 Turbo for **$0.04/hour**, equivalent to about
**$0.00067 per audio minute**. It accepts uploaded audio rather than providing
the same partial-result streaming behavior as a voice-agent-specific STT
service, and it has a minimum billed duration of ten seconds per request.

It can work well when the application detects the end of each caller turn and
submits that completed audio segment. It is less appropriate when fluid
barge-in and continuously updated partial transcripts are important.

Source: [Groq speech-to-text documentation](https://console.groq.com/docs/speech-to-text)

## Self-hosting

At sufficiently high volume, speech recognition, text generation, and speech
synthesis can all run locally using combinations such as Whisper or
faster-whisper, a small local LLM, and Piper or Kokoro TTS. Marginal API cost
then approaches zero, but the system still incurs:

- PSTN or SIP carrier charges
- Compute and GPU hosting
- Scaling and concurrency engineering
- Monitoring and failover
- More work to achieve good endpoint detection, interruption handling, and
  conversational latency

This route is usually economical only when call volume is high enough to keep
the compute busy or when local processing and privacy are strategic
requirements.

## Practical recommendations

1. For the fastest inexpensive launch, test **Plivo's managed $0.03/minute
   agent** and verify the exact included model and voice.
2. For natural speech-to-speech at low cost, test a SIP carrier with
   **OpenAI GPT-Realtime-2.1 mini**.
3. For the lowest predictable cost at meaningful volume, build a modular
   **Plivo or Telnyx + Pipecat + AssemblyAI + small LLM + inexpensive neural
   TTS** pipeline.
4. Benchmark using real telephone calls rather than browser audio. PSTN audio
   is narrowband, so much of the benefit of expensive studio-quality TTS can
   disappear over the phone.

## Cost-control considerations

- Use voice activity detection so silence is not unnecessarily processed by
  services that bill by audio duration.
- Stop TTS generation immediately when the caller interrupts.
- Stream LLM output into TTS by phrase or clause to reduce response latency.
- Bound, summarize, or compress conversation history so context charges do
  not grow throughout a long call.
- Check whether the STT vendor bills only audio or the entire open session.
- Account for per-minute rounding, call transfers, recording, phone-number
  rental, and separate SIP or media-stream legs.
- Keep responses concise. This reduces LLM output, TTS generation, and the
  percentage of each call occupied by the AI.
