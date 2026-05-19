"use strict";

const encodeVarint = (value) => {
  const v = BigInt(value);
  if (v < 128n) {
    return new Uint8Array([Number(v)]);
  }
  if (v < 16384n) {
    return new Uint8Array([0x80 | Number(v >> 8n), Number(v & 0xffn)]);
  }
  if (v < 2097152n) {
    return new Uint8Array([
      0xc0 | Number(v >> 16n),
      Number((v >> 8n) & 0xffn),
      Number(v & 0xffn),
    ]);
  }
  if (v < 268435456n) {
    return new Uint8Array([
      0xe0 | Number(v >> 24n),
      Number((v >> 16n) & 0xffn),
      Number((v >> 8n) & 0xffn),
      Number(v & 0xffn),
    ]);
  }
  throw new Error("varint too large: " + value);
};

const concat = (...arrays) => {
  const total = arrays.reduce((n, a) => n + a.length, 0);
  const out = new Uint8Array(total);
  let pos = 0;
  for (const a of arrays) {
    out.set(a, pos);
    pos += a.length;
  }
  return out;
};

const encodeString = (s) => {
  const b = new TextEncoder().encode(s);
  return concat(encodeVarint(b.length), b);
};

const encodeNamespace = (parts) => {
  let b = encodeVarint(parts.length);
  for (const p of parts) b = concat(b, encodeString(p));
  return b;
};

const encodeMsg = (type, payload) =>
  concat(
    encodeVarint(type),
    new Uint8Array([(payload.length >> 8) & 0xff, payload.length & 0xff]),
    payload,
  );

const isKeyFrame = (codec, data) => {
  if (codec.startsWith("avc1")) {
    // AVCC: scan length-prefixed NALUs for IDR (type 5)
    let i = 0;
    while (i + 4 <= data.length) {
      const len =
        ((data[i] << 24) |
          (data[i + 1] << 16) |
          (data[i + 2] << 8) |
          data[i + 3]) >>>
        0;
      i += 4;
      if (i < data.length && (data[i] & 0x1f) === 5) {
        return true;
      }
      i += len;
    }
    return false;
  }
  if (codec.startsWith("hvc1")) {
    // HEVC: NAL type in bits [9:15] of the 2-byte NAL header
    let i = 0;
    while (i + 4 <= data.length) {
      const len =
        ((data[i] << 24) |
          (data[i + 1] << 16) |
          (data[i + 2] << 8) |
          data[i + 3]) >>>
        0;
      i += 4;
      if (i < data.length) {
        const nalType = (data[i] >> 1) & 0x3f;
        if (nalType >= 16 && nalType <= 23) {
          return true; // IRAP
        }
      }
      i += len;
    }
    return false;
  }
  return true; // assume key for AV1/VP9/VP8
};

const base64ToBuffer = (b64) => {
  const bin = atob(b64);
  const buf = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i);
  return buf;
};

const bytesEqual = (a, b) => {
  if (a.length !== b.length) {
    return false;
  }
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) {
      return false;
    }
  }
  return true;
};

const buildAvcC = (spsNalus, ppsNalus) => {
  const sps = spsNalus[0];
  const parts = [
    new Uint8Array([1, sps[1], sps[2], sps[3], 0xff]),
    new Uint8Array([0xe0 | spsNalus.length]),
  ];
  for (const s of spsNalus) {
    parts.push(new Uint8Array([(s.length >> 8) & 0xff, s.length & 0xff]));
    parts.push(s);
  }
  parts.push(new Uint8Array([ppsNalus.length]));
  for (const p of ppsNalus) {
    parts.push(new Uint8Array([(p.length >> 8) & 0xff, p.length & 0xff]));
    parts.push(p);
  }
  return concat(...parts);
};

const buildHvcC = (vpsNalus, spsNalus, ppsNalus) => {
  const hdr = new Uint8Array(23);
  hdr[0] = 1; // configurationVersion
  hdr[13] = 0xf0; // reserved
  hdr[15] = 0xfc; // reserved | parallelismType=0
  hdr[16] = 0xfc; // reserved | chroma_format_idc=0
  hdr[17] = 0xf8; // reserved | bit_depth_luma_minus8=0
  hdr[18] = 0xf8; // reserved | bit_depth_chroma_minus8=0
  hdr[21] = 0x03; // lengthSizeMinusOne=3
  const arrays = [];
  const addArray = (type, nalus) => {
    if (!nalus.length) {
      return;
    }
    const parts = [new Uint8Array([0x80 | type, 0x00, nalus.length])];
    for (const n of nalus) {
      parts.push(new Uint8Array([(n.length >> 8) & 0xff, n.length & 0xff]));
      parts.push(n);
    }
    arrays.push(concat(...parts));
  };
  addArray(32, vpsNalus);
  addArray(33, spsNalus);
  addArray(34, ppsNalus);
  hdr[22] = arrays.length;
  return concat(hdr, ...arrays);
};

const extractInStreamParams = (codec, data) => {
  if (codec.startsWith("avc1") || codec.startsWith("avc3")) {
    const sps = [],
      pps = [];
    let i = 0;
    while (i + 4 <= data.length) {
      const len =
        ((data[i] << 24) |
          (data[i + 1] << 16) |
          (data[i + 2] << 8) |
          data[i + 3]) >>>
        0;
      i += 4;
      if (i + len > data.length) {
        break;
      }
      const t = data[i] & 0x1f;
      if (t === 7) {
        sps.push(data.slice(i, i + len));
      } else if (t === 8) {
        pps.push(data.slice(i, i + len));
      }
      i += len;
    }
    return sps.length && pps.length ? buildAvcC(sps, pps) : null;
  }
  if (codec.startsWith("hvc1") || codec.startsWith("hev1")) {
    const vps = [],
      sps = [],
      pps = [];
    let i = 0;
    while (i + 4 <= data.length) {
      const len =
        ((data[i] << 24) |
          (data[i + 1] << 16) |
          (data[i + 2] << 8) |
          data[i + 3]) >>>
        0;
      i += 4;
      if (i + len > data.length) {
        break;
      }
      const t = (data[i] >> 1) & 0x3f;
      if (t === 32) {
        vps.push(data.slice(i, i + len));
      } else if (t === 33) {
        sps.push(data.slice(i, i + len));
      } else if (t === 34) {
        pps.push(data.slice(i, i + len));
      }
      i += len;
    }
    return sps.length ? buildHvcC(vps, sps, pps) : null;
  }
  return null;
};

class StreamReader {
  #reader = null;
  #buf = new Uint8Array(0);

  constructor(src) {
    if (src instanceof Uint8Array) {
      this.#buf = src;
    } else {
      this.#reader = src.getReader();
    }
  }

  async #fill() {
    if (!this.#reader) {
      throw new Error("stream ended");
    }
    const { value, done } = await this.#reader.read();
    if (done) {
      throw new Error("stream ended");
    }
    const next = new Uint8Array(this.#buf.length + value.length);
    next.set(this.#buf);
    next.set(value, this.#buf.length);
    this.#buf = next;
  }

  async readBytes(n) {
    while (this.#buf.length < n) await this.#fill();
    const out = this.#buf.slice(0, n);
    this.#buf = this.#buf.slice(n);
    return out;
  }

  async readVarint() {
    while (this.#buf.length < 1) await this.#fill();
    const b = this.#buf[0];
    let size;
    if ((b & 0x80) === 0) {
      size = 1;
    } else if ((b & 0xc0) === 0x80) {
      size = 2;
    } else if ((b & 0xe0) === 0xc0) {
      size = 3;
    } else if ((b & 0xf0) === 0xe0) {
      size = 4;
    } else if ((b & 0xf8) === 0xf0) {
      size = 5;
    } else {
      throw new Error("unsupported varint: 0x" + b.toString(16));
    }
    const d = await this.readBytes(size);
    switch (size) {
      case 1:
        return BigInt(d[0]);
      case 2:
        return BigInt(((d[0] & 0x3f) << 8) | d[1]);
      case 3:
        return BigInt(((d[0] & 0x1f) << 16) | (d[1] << 8) | d[2]);
      case 4:
        return BigInt(
          ((d[0] & 0x0f) << 24) | (d[1] << 16) | (d[2] << 8) | d[3],
        );
      case 5:
        return (
          (BigInt(d[0] & 0x07) << 32n) |
          (BigInt(d[1]) << 24n) |
          (BigInt(d[2]) << 16n) |
          (BigInt(d[3]) << 8n) |
          BigInt(d[4])
        );
    }
  }

  async readU16() {
    const d = await this.readBytes(2);
    return (d[0] << 8) | d[1];
  }
}

/**
 * @callback OnError
 * @param {string} err - error.
 */

/**
 * @callback OnSubscribed
 */

/**
 * @typedef Conf
 * @type {object}
 * @property {HTMLElement} videoElement - element where the video canvas will be appended.
 * @property {OnError} onError - called when there's an error.
 * @property {OnSubscribed} onSubscribed - called when track subscription is successful.
 */

/** Media-over-QUIC reader. */
class MediaMTXMoQReader {
  static #RETRY_PAUSE = 2000;

  static MOQT_VERSION = "moqt-18";

  static SETUP_TYPE = 0x2f00n;
  static MSG_SUBSCRIBE = 0x03n;
  static MSG_SUBSCRIBE_OK = 0x04n;
  static SUBGROUP_TYPE = 0x30n;

  static VIDEO_REQUEST_ID = BigInt(10);
  static AUDIO_REQUEST_ID = BigInt(11);

  #conf;
  #state = "running";
  #restartTimeout = null;
  #wt = null;
  #namespace = window.location.pathname
    .replace(/^\/|\/$/g, "")
    .split("/")
    .filter(Boolean);
  #fingerprint = null;
  #catalog = null;
  #uniQueue = [];
  #uniWaiters = [];
  #videoTrack = null;
  #videoParams = null;
  #videoCanvas = null;
  #videoDecoder = null;
  #audioTrack = null;
  #audioCtx = null;
  #audioInitialized = false;
  #audioStartPTS = null;
  #audioStartSystem = null;
  #audioDecoder = null;

  constructor(conf) {
    this.#conf = conf;
    this.#start();
  }

  #start() {
    this.#fetchFingerprint()
      .then(() => this.#connect())
      .then(() => this.#setup())
      .then(() => this.#subscribeCatalog())
      .then(() => this.#subscribeAllTracks())
      .then(() => this.#drainDataStreams())
      .catch((err) => this.#handleError(err.message));
  }

  /** @param {string} err */
  #handleError(err) {
    if (this.#state === "running") {
      if (this.#wt !== null) {
        this.#wt.close();
        this.#wt = null;
      }

      if (this.#videoDecoder !== null) {
        this.#videoDecoder.close();
        this.#videoDecoder = null;
      }

      if (this.#videoCanvas !== null) {
        this.#videoCanvas.remove();
        this.#videoCanvas = null;
      }

      if (this.#audioDecoder !== null) {
        this.#audioDecoder.close();
        this.#audioDecoder = null;
      }

      if (this.#audioCtx !== null) {
        this.#audioCtx.close();
        this.#audioCtx = null;
      }

      this.#uniQueue = [];
      for (const w of this.#uniWaiters) {
        w.reject(new Error("restarting"));
      }
      this.#uniWaiters = [];
      this.#videoTrack = null;
      this.#videoParams = null;
      this.#audioTrack = null;
      this.#audioInitialized = false;
      this.#audioStartPTS = null;
      this.#audioStartSystem = null;

      this.#restartTimeout = window.setTimeout(
        () => this.#restart(),
        MediaMTXMoQReader.#RETRY_PAUSE,
      );

      if (this.#conf.onError !== undefined) {
        this.#conf.onError(`${err}, retrying in some seconds`);
      }

      this.#state = "restarting";
    }
  }

  #restart() {
    this.#restartTimeout = null;
    this.#state = "running";
    this.#start();
  }

  async #fetchFingerprint() {
    const hex = await fetch("fingerprint").then((r) => r.text());
    this.#fingerprint = new Uint8Array(hex.length / 2);
    for (let i = 0; i < this.#fingerprint.length; i++)
      this.#fingerprint[i] = parseInt(hex.slice(2 * i, 2 * i + 2), 16);
  }

  async #connect() {
    this.#wt = new WebTransport(
      new URL("moq", window.location.href) + window.location.search,
      {
        serverCertificateHashes: [
          { algorithm: "sha-256", value: this.#fingerprint.buffer },
        ],
        protocols: [MediaMTXMoQReader.MOQT_VERSION],
      },
    );
    await this.#wt.ready;
    console.log("connected");

    this.#wt.closed
      .then(() => this.#handleError("connection closed"))
      .catch((err) => this.#handleError(err.message));

    this.#acceptUniStreams().catch((err) => this.#handleError(err.message));
  }

  async #acceptUniStreams() {
    const reader = this.#wt.incomingUnidirectionalStreams.getReader();
    for (;;) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }
      if (this.#uniWaiters.length > 0) {
        this.#uniWaiters.shift().resolve(value);
      } else {
        this.#uniQueue.push(value);
      }
    }
  }

  #nextUni() {
    return new Promise((resolve, reject) => {
      if (this.#uniQueue.length > 0) {
        resolve(this.#uniQueue.shift());
      } else {
        this.#uniWaiters.push({ resolve, reject });
      }
    });
  }

  async #setup() {
    const tx = await this.#wt.createUnidirectionalStream();
    const w = tx.getWriter();
    await w.write(encodeVarint(MediaMTXMoQReader.SETUP_TYPE));
    await w.write(new Uint8Array([0x00, 0x00]));
    w.releaseLock();

    const rx = new StreamReader(await this.#nextUni());
    const t = await rx.readVarint();
    if (t !== MediaMTXMoQReader.SETUP_TYPE) {
      throw new Error("unexpected setup type 0x" + t.toString(16));
    }
    await rx.readBytes(await rx.readU16());
    console.log("setup ok");
  }

  async #subscribeCatalog() {
    const bidi = await this.#wt.createBidirectionalStream();
    const w = bidi.writable.getWriter();
    const r = new StreamReader(bidi.readable);

    await w.write(
      encodeMsg(
        MediaMTXMoQReader.MSG_SUBSCRIBE,
        concat(
          encodeVarint(0),
          encodeNamespace(this.#namespace),
          encodeString(".catalog"),
          encodeVarint(0),
        ),
      ),
    );
    w.releaseLock();

    const t = await r.readVarint();
    if (t !== MediaMTXMoQReader.MSG_SUBSCRIBE_OK) {
      throw new Error("expected SUBSCRIBE_OK, got 0x" + t.toString(16));
    }
    await r.readBytes(await r.readU16());

    const payload = await this.#readSubgroupPayload(await this.#nextUni());
    this.#catalog = JSON.parse(new TextDecoder().decode(payload));

    console.log("catalog:", this.#catalog);
  }

  async #readSubgroupPayload(readable) {
    const r = new StreamReader(readable);
    if ((await r.readVarint()) !== MediaMTXMoQReader.SUBGROUP_TYPE) {
      throw new Error("not a subgroup stream");
    }
    await r.readVarint(); // trackAlias
    await r.readVarint(); // groupId
    const chunks = [];
    for (;;) {
      await r.readVarint(); // idDelta
      const len = await r.readVarint();
      if (len === 0n) {
        break;
      }
      chunks.push(await r.readBytes(Number(len)));
    }
    return concat(...chunks);
  }

  async #subscribeAllTracks() {
    const promises = [];

    for (let i = 0; i < this.#catalog.tracks.length; i++) {
      const track = this.#catalog.tracks[i];
      const isVideo = /^(avc1|hvc1|av01|vp09|vp8)/.test(track.codec);

      if (isVideo && this.#videoTrack === null) {
        this.#videoTrack = track;
        promises.push(
          this.#subscribeTrack(MediaMTXMoQReader.VIDEO_REQUEST_ID, track),
        );
      } else if (!isVideo && this.#audioTrack === null) {
        this.#audioTrack = track;
        promises.push(
          this.#subscribeTrack(MediaMTXMoQReader.AUDIO_REQUEST_ID, track),
        );
      }
    }

    await Promise.all(promises);

    if (this.#conf.onSubscribed !== undefined) {
      this.#conf.onSubscribed();
    }
  }

  async #subscribeTrack(requestId, track) {
    const bidi = await this.#wt.createBidirectionalStream();
    const w = bidi.writable.getWriter();
    const r = new StreamReader(bidi.readable);

    await w.write(
      encodeMsg(
        MediaMTXMoQReader.MSG_SUBSCRIBE,
        concat(
          encodeVarint(requestId),
          encodeNamespace(this.#namespace),
          encodeString(track.name),
          encodeVarint(0),
        ),
      ),
    );
    w.releaseLock();

    const t = await r.readVarint();
    if (t !== MediaMTXMoQReader.MSG_SUBSCRIBE_OK) {
      throw new Error("expected SUBSCRIBE_OK, got 0x" + t.toString(16));
    }
    await r.readBytes(await r.readU16());

    if (requestId === MediaMTXMoQReader.VIDEO_REQUEST_ID) {
      this.#videoCanvas = document.createElement("canvas");
      this.#conf.videoElement.appendChild(this.#videoCanvas);
      const ctx2d = this.#videoCanvas.getContext("2d");

      this.#videoDecoder = new VideoDecoder({
        output: (frame) => {
          if (
            this.#videoCanvas.width !== frame.displayWidth ||
            this.#videoCanvas.height !== frame.displayHeight
          ) {
            this.#videoCanvas.width = frame.displayWidth;
            this.#videoCanvas.height = frame.displayHeight;
          }
          ctx2d.drawImage(
            frame,
            0,
            0,
            this.#videoCanvas.width,
            this.#videoCanvas.height,
          );
          frame.close();
        },
        error: (err) => console.error(err.message),
      });

      const config = {
        codec: track.codec,
        optimizeForLatency: true,
      };
      if (track.initData) {
        this.#videoParams = base64ToBuffer(track.initData);
        config.description = this.#videoParams;
      }
      this.#videoDecoder.configure(config);
    } else {
      this.#audioCtx = new AudioContext();

      this.#audioDecoder = new AudioDecoder({
        output: (data) => {
          //try {
          const buf = this.#audioCtx.createBuffer(
            data.numberOfChannels,
            data.numberOfFrames,
            data.sampleRate,
          );
          for (let ch = 0; ch < data.numberOfChannels; ch++) {
            data.copyTo(buf.getChannelData(ch), {
              planeIndex: ch,
              format: "f32-planar",
            });
          }

          const src = this.#audioCtx.createBufferSource();
          src.buffer = buf;
          src.connect(this.#audioCtx.destination);

          const when =
            this.#audioStartSystem +
            data.timestamp / this.#audioTrack.samplerate;
          src.start(when);
          //src.start(Math.max(when, this.#audioCtx.currentTime));
          // src.start();
          //} catch(err) {
          //    console.error(err);
          //} finally {
          data.close();
          //}
        },
        error: (err) => console.error(err),
      });

      const config = {
        codec: track.codec,
        sampleRate: track.samplerate,
        numberOfChannels: track.channels,
      };
      if (track.initData) {
        config.description = base64ToBuffer(track.initData);
      }
      this.#audioDecoder.configure(config);
    }

    console.log("subscribed track " + track.name + " (" + track.codec + ")");
  }

  async #drainDataStreams() {
    for (;;) {
      const stream = await this.#nextUni();
      this.#handleDataStream(stream).catch((err) =>
        this.#handleError(err.message),
      );
    }
  }

  async #handleDataStream(readable) {
    const r = new StreamReader(readable);
    const streamType = await r.readVarint();
    if (streamType !== MediaMTXMoQReader.SUBGROUP_TYPE) {
      return;
    }

    const requestId = await r.readVarint();
    const groupId = await r.readVarint();

    const chunks = [];
    for (;;) {
      await r.readVarint(); // idDelta
      const len = await r.readVarint();
      if (len === 0n) {
        break;
      }
      chunks.push(await r.readBytes(Number(len)));
    }
    if (chunks.length === 0) {
      return;
    }

    const data = concat(...chunks);

    if (requestId === MediaMTXMoQReader.VIDEO_REQUEST_ID) {
      if (isKeyFrame(this.#videoTrack.codec, data)) {
        const newDesc = extractInStreamParams(this.#videoTrack.codec, data);
        if (
          newDesc !== null &&
          (this.#videoParams === null ||
            !bytesEqual(newDesc, this.#videoParams))
        ) {
          this.#videoParams = newDesc;
          this.#videoDecoder.configure({
            codec: this.#videoTrack.codec,
            optimizeForLatency: true,
            description: newDesc,
          });
          console.log("video params updated");
        }
      }

      const ts = performance.now() * 1000;
      this.#videoDecoder.decode(
        new EncodedVideoChunk({ type: "key", timestamp: ts, data }),
      );
    } else {
      if (!this.#audioInitialized) {
        this.#audioInitialized = true;
        this.#audioStartPTS = groupId;
        this.#audioStartSystem = this.#audioCtx.currentTime;
      }

      const ts = Number(groupId - this.#audioStartPTS);
      //const ts = performance.now() * 1000;

      this.#audioDecoder.decode(
        new EncodedAudioChunk({ type: "key", timestamp: ts, data }),
      );
    }
  }
}
