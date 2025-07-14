;(function (_, D) {
  typeof exports == "object" && typeof module < "u"
    ? D(exports)
    : typeof define == "function" && define.amd
      ? define(["exports"], D)
      : ((_ = typeof globalThis < "u" ? globalThis : _ || self), D((_.UnicornStudio = {})))
})(this, function (_) {
  "use strict"
  var is = Object.defineProperty
  var rs = (_, D, g) => (D in _ ? is(_, D, { enumerable: !0, configurable: !0, writable: !0, value: g }) : (_[D] = g))
  var L = (_, D, g) => (rs(_, typeof D != "symbol" ? D + "" : D, g), g)
  let D = 0
  function g() {
    if (!(D > 100)) {
      if (D === 100) console.warn("Curtains: too many warnings thrown, stop logging.")
      else {
        const n = Array.prototype.slice.call(arguments)
        console.warn.apply(console, n)
      }
      D++
    }
  }
  function F() {
    const n = Array.prototype.slice.call(arguments)
    console.error.apply(console, n)
  }
  function Se() {
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (n) => {
      let e = (Math.random() * 16) | 0
      return (n === "x" ? e : (e & 3) | 8).toString(16).toUpperCase()
    })
  }
  function Q(n) {
    return (n & (n - 1)) === 0
  }
  function Ke(n, e, t) {
    return (1 - t) * n + t * e
  }
  let et = class {
    constructor(e) {
      if (((this.type = "Scene"), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        F(this.type + ": Renderer WebGL context is undefined", e)
        return
      }
      ;((this.renderer = e), (this.gl = e.gl), this.initStacks())
    }
    initStacks() {
      this.stacks = { pingPong: [], renderTargets: [], opaque: [], transparent: [], renderPasses: [], scenePasses: [] }
    }
    resetPlaneStacks() {
      ;((this.stacks.pingPong = []),
        (this.stacks.renderTargets = []),
        (this.stacks.opaque = []),
        (this.stacks.transparent = []))
      for (let e = 0; e < this.renderer.planes.length; e++) this.addPlane(this.renderer.planes[e])
    }
    resetShaderPassStacks() {
      ;((this.stacks.scenePasses = []), (this.stacks.renderPasses = []))
      for (let e = 0; e < this.renderer.shaderPasses.length; e++)
        ((this.renderer.shaderPasses[e].index = e),
          this.renderer.shaderPasses[e]._isScenePass
            ? this.stacks.scenePasses.push(this.renderer.shaderPasses[e])
            : this.stacks.renderPasses.push(this.renderer.shaderPasses[e]))
      this.stacks.scenePasses.length === 0 && (this.renderer.state.scenePassIndex = null)
    }
    addToRenderTargetsStack(e) {
      const t = this.renderer.planes.filter((i) => i.type !== "PingPongPlane" && i.target && i.uuid !== e.uuid)
      let s = -1
      if (e.target._depth) {
        for (let i = t.length - 1; i >= 0; i--)
          if (t[i].target.uuid === e.target.uuid) {
            s = i + 1
            break
          }
      } else s = t.findIndex((i) => i.target.uuid === e.target.uuid)
      ;((s = Math.max(0, s)),
        t.splice(s, 0, e),
        e.target._depth
          ? (t.sort((i, r) => i.index - r.index), t.sort((i, r) => r.renderOrder - i.renderOrder))
          : (t.sort((i, r) => r.index - i.index), t.sort((i, r) => i.renderOrder - r.renderOrder)),
        t.sort((i, r) => i.target.index - r.target.index),
        (this.stacks.renderTargets = t))
    }
    addToRegularPlaneStack(e) {
      const t = this.renderer.planes.filter(
        (i) => i.type !== "PingPongPlane" && !i.target && i._transparent === e._transparent && i.uuid !== e.uuid,
      )
      let s = -1
      for (let i = t.length - 1; i >= 0; i--)
        if (t[i]._geometry.definition.id === e._geometry.definition.id) {
          s = i + 1
          break
        }
      return ((s = Math.max(0, s)), t.splice(s, 0, e), t.sort((i, r) => i.index - r.index), t)
    }
    addPlane(e) {
      if (e.type === "PingPongPlane") this.stacks.pingPong.push(e)
      else if (e.target) this.addToRenderTargetsStack(e)
      else if (e._transparent) {
        const t = this.addToRegularPlaneStack(e)
        ;(t.sort((s, i) => i.relativeTranslation.z - s.relativeTranslation.z),
          t.sort((s, i) => i.renderOrder - s.renderOrder),
          (this.stacks.transparent = t))
      } else {
        const t = this.addToRegularPlaneStack(e)
        ;(t.sort((s, i) => i.renderOrder - s.renderOrder), (this.stacks.opaque = t))
      }
    }
    removePlane(e) {
      e.type === "PingPongPlane"
        ? (this.stacks.pingPong = this.stacks.pingPong.filter((t) => t.uuid !== e.uuid))
        : e.target
          ? (this.stacks.renderTargets = this.stacks.renderTargets.filter((t) => t.uuid !== e.uuid))
          : e._transparent
            ? (this.stacks.transparent = this.stacks.transparent.filter((t) => t.uuid !== e.uuid))
            : (this.stacks.opaque = this.stacks.opaque.filter((t) => t.uuid !== e.uuid))
    }
    setPlaneRenderOrder(e) {
      if (e.type === "ShaderPass")
        this.sortShaderPassStack(e._isScenePass ? this.stacks.scenePasses : this.stacks.renderPasses)
      else if (e.type === "PingPongPlane") return
      if (e.target)
        (e.target._depth
          ? (this.stacks.renderTargets.sort((t, s) => t.index - s.index),
            this.stacks.renderTargets.sort((t, s) => s.renderOrder - t.renderOrder))
          : (this.stacks.renderTargets.sort((t, s) => s.index - t.index),
            this.stacks.renderTargets.sort((t, s) => t.renderOrder - s.renderOrder)),
          this.stacks.renderTargets.sort((t, s) => t.target.index - s.target.index))
      else {
        const t = e._transparent ? this.stacks.transparent : this.stacks.opaque,
          s = this.stacks.scenePasses.find((i, r) => i._isScenePass && !i._depth && r === 0)
        !this.renderer.depth || s
          ? (t.sort((i, r) => r.index - i.index),
            e._transparent && t.sort((i, r) => i.relativeTranslation.z - r.relativeTranslation.z),
            t.sort((i, r) => i.renderOrder - r.renderOrder))
          : (t.sort((i, r) => i.index - r.index),
            e._transparent && t.sort((i, r) => r.relativeTranslation.z - i.relativeTranslation.z),
            t.sort((i, r) => r.renderOrder - i.renderOrder))
      }
    }
    addShaderPass(e) {
      e._isScenePass
        ? (this.stacks.scenePasses.push(e), this.sortShaderPassStack(this.stacks.scenePasses))
        : (this.stacks.renderPasses.push(e), this.sortShaderPassStack(this.stacks.renderPasses))
    }
    removeShaderPass(e) {
      this.resetShaderPassStacks()
    }
    sortShaderPassStack(e) {
      ;(e.sort((t, s) => t.index - s.index), e.sort((t, s) => t.renderOrder - s.renderOrder))
    }
    enableShaderPass() {
      this.stacks.scenePasses.length &&
        this.stacks.renderPasses.length === 0 &&
        this.renderer.planes.length &&
        ((this.renderer.state.scenePassIndex = 0), this.renderer.bindFrameBuffer(this.stacks.scenePasses[0].target))
    }
    drawRenderPasses() {
      this.stacks.scenePasses.length &&
        this.stacks.renderPasses.length &&
        this.renderer.planes.length &&
        ((this.renderer.state.scenePassIndex = 0), this.renderer.bindFrameBuffer(this.stacks.scenePasses[0].target))
      for (let e = 0; e < this.stacks.renderPasses.length; e++)
        (this.stacks.renderPasses[e]._startDrawing(), this.renderer.clearDepth())
    }
    drawScenePasses() {
      for (let e = 0; e < this.stacks.scenePasses.length; e++) this.stacks.scenePasses[e]._startDrawing()
    }
    drawPingPongStack() {
      for (let e = 0; e < this.stacks.pingPong.length; e++) {
        const t = this.stacks.pingPong[e]
        t && t._startDrawing()
      }
    }
    drawStack(e) {
      for (let t = 0; t < this.stacks[e].length; t++) {
        const s = this.stacks[e][t]
        s && s._startDrawing()
      }
    }
    draw() {
      ;(this.drawPingPongStack(),
        this.enableShaderPass(),
        this.drawStack("renderTargets"),
        this.drawRenderPasses(),
        this.renderer.setBlending(!1),
        this.drawStack("opaque"),
        this.stacks.transparent.length && (this.renderer.setBlending(!0), this.drawStack("transparent")),
        this.drawScenePasses())
    }
  }
  class tt {
    constructor() {
      ;((this.geometries = []), this.clear())
    }
    clear() {
      ;((this.textures = []), (this.programs = []))
    }
    getGeometryFromID(e) {
      return this.geometries.find((t) => t.id === e)
    }
    addGeometry(e, t, s) {
      this.geometries.push({ id: e, vertices: t, uvs: s })
    }
    isSameShader(e, t) {
      return e.localeCompare(t) === 0
    }
    getProgramFromShaders(e, t) {
      return this.programs.find((s) => this.isSameShader(s.vsCode, e) && this.isSameShader(s.fsCode, t))
    }
    addProgram(e) {
      this.programs.push(e)
    }
    getTextureFromSource(e) {
      const t = typeof e == "string" ? e : e.src
      return this.textures.find((s) => s.source && s.source.src === t)
    }
    addTexture(e) {
      this.getTextureFromSource(e.source) || this.textures.push(e)
    }
    removeTexture(e) {
      this.textures = this.textures.filter((t) => t.uuid !== e.uuid)
    }
  }
  class st {
    constructor() {
      this.clear()
    }
    clear() {
      this.queue = []
    }
    add(e, t = !1) {
      const s = { callback: e, keep: t, timeout: null }
      return (
        (s.timeout = setTimeout(() => {
          this.queue.push(s)
        }, 0)),
        s
      )
    }
    execute() {
      ;(this.queue.map((e) => {
        ;(e.callback && e.callback(), clearTimeout(this.queue.timeout))
      }),
        (this.queue = this.queue.filter((e) => e.keep)))
    }
  }
  class it {
    constructor({
      alpha: e,
      antialias: t,
      premultipliedAlpha: s,
      depth: i,
      failIfMajorPerformanceCaveat: r,
      preserveDrawingBuffer: a,
      stencil: h,
      container: o,
      pixelRatio: l,
      renderingScale: d,
      production: u,
      onError: c,
      onSuccess: f,
      onContextLost: p,
      onContextRestored: m,
      onDisposed: T,
      onSceneChange: S,
    }) {
      ;((this.type = "Renderer"),
        (this.alpha = e),
        (this.antialias = t),
        (this.premultipliedAlpha = s),
        (this.depth = i),
        (this.failIfMajorPerformanceCaveat = r),
        (this.preserveDrawingBuffer = a),
        (this.stencil = h),
        (this.container = o),
        (this.pixelRatio = l),
        (this._renderingScale = d),
        (this.production = u),
        (this.onError = c),
        (this.onSuccess = f),
        (this.onContextLost = p),
        (this.onContextRestored = m),
        (this.onDisposed = T),
        (this.onSceneChange = S),
        this.initState(),
        (this.canvas = document.createElement("canvas")))
      const R = {
        alpha: this.alpha,
        premultipliedAlpha: this.premultipliedAlpha,
        antialias: this.antialias,
        depth: this.depth,
        failIfMajorPerformanceCaveat: this.failIfMajorPerformanceCaveat,
        preserveDrawingBuffer: this.preserveDrawingBuffer,
        stencil: this.stencil,
      }
      if (
        ((this.gl = this.canvas.getContext("webgl2", R)),
        (this._isWebGL2 = !!this.gl),
        this.gl || (this.gl = this.canvas.getContext("webgl", R) || this.canvas.getContext("experimental-webgl", R)),
        this.gl)
      )
        this.onSuccess && this.onSuccess()
      else {
        ;(this.production || g(this.type + ": WebGL context could not be created"),
          (this.state.isActive = !1),
          this.onError && this.onError())
        return
      }
      this.initRenderer()
    }
    initState() {
      this.state = {
        isActive: !0,
        isContextLost: !0,
        drawingEnabled: !0,
        forceRender: !1,
        currentProgramID: null,
        currentGeometryID: null,
        forceBufferUpdate: !1,
        depthTest: null,
        blending: null,
        cullFace: null,
        frameBufferID: null,
        scenePassIndex: null,
        activeTexture: null,
        unpackAlignment: null,
        flipY: null,
        premultiplyAlpha: null,
      }
    }
    initCallbackQueueManager() {
      this.nextRender = new st()
    }
    initRenderer() {
      ;((this.planes = []),
        (this.renderTargets = []),
        (this.shaderPasses = []),
        (this.state.isContextLost = !1),
        (this.state.maxTextureSize = this.gl.getParameter(this.gl.MAX_TEXTURE_SIZE)),
        this.initCallbackQueueManager(),
        this.setBlendFunc(),
        this.setDepthFunc(),
        this.setDepthTest(!0),
        (this.cache = new tt()),
        (this.scene = new et(this)),
        this.getExtensions(),
        (this._contextLostHandler = this.contextLost.bind(this)),
        this.canvas.addEventListener("webglcontextlost", this._contextLostHandler, !1),
        (this._contextRestoredHandler = this.contextRestored.bind(this)),
        this.canvas.addEventListener("webglcontextrestored", this._contextRestoredHandler, !1))
    }
    getExtensions() {
      ;((this.extensions = []),
        this._isWebGL2
          ? ((this.extensions.EXT_color_buffer_float = this.gl.getExtension("EXT_color_buffer_float")),
            (this.extensions.OES_texture_float_linear = this.gl.getExtension("OES_texture_float_linear")),
            (this.extensions.EXT_texture_filter_anisotropic = this.gl.getExtension("EXT_texture_filter_anisotropic")),
            (this.extensions.WEBGL_lose_context = this.gl.getExtension("WEBGL_lose_context")))
          : ((this.extensions.OES_vertex_array_object = this.gl.getExtension("OES_vertex_array_object")),
            (this.extensions.OES_texture_float = this.gl.getExtension("OES_texture_float")),
            (this.extensions.OES_texture_float_linear = this.gl.getExtension("OES_texture_float_linear")),
            (this.extensions.OES_texture_half_float = this.gl.getExtension("OES_texture_half_float")),
            (this.extensions.OES_texture_half_float_linear = this.gl.getExtension("OES_texture_half_float_linear")),
            (this.extensions.EXT_texture_filter_anisotropic = this.gl.getExtension("EXT_texture_filter_anisotropic")),
            (this.extensions.OES_element_index_uint = this.gl.getExtension("OES_element_index_uint")),
            (this.extensions.OES_standard_derivatives = this.gl.getExtension("OES_standard_derivatives")),
            (this.extensions.EXT_sRGB = this.gl.getExtension("EXT_sRGB")),
            (this.extensions.WEBGL_depth_texture = this.gl.getExtension("WEBGL_depth_texture")),
            (this.extensions.WEBGL_draw_buffers = this.gl.getExtension("WEBGL_draw_buffers")),
            (this.extensions.WEBGL_lose_context = this.gl.getExtension("WEBGL_lose_context"))))
    }
    contextLost(e) {
      ;((this.state.isContextLost = !0),
        this.state.isActive &&
          (e.preventDefault(), this.nextRender.add(() => this.onContextLost && this.onContextLost())))
    }
    restoreContext() {
      this.state.isActive &&
        (this.initState(),
        this.gl && this.extensions.WEBGL_lose_context
          ? this.extensions.WEBGL_lose_context.restoreContext()
          : (!this.gl && !this.production
              ? g(this.type + ": Could not restore the context because the context is not defined")
              : !this.extensions.WEBGL_lose_context &&
                !this.production &&
                g(this.type + ": Could not restore the context because the restore context extension is not defined"),
            this.onError && this.onError()))
    }
    isContextexFullyRestored() {
      let e = !0
      for (let t = 0; t < this.renderTargets.length; t++) {
        this.renderTargets[t].textures[0]._canDraw || (e = !1)
        break
      }
      if (e)
        for (let t = 0; t < this.planes.length; t++)
          if (this.planes[t]._canDraw) {
            for (let s = 0; s < this.planes[t].textures.length; s++)
              if (!this.planes[t].textures[s]._canDraw) {
                e = !1
                break
              }
          } else {
            e = !1
            break
          }
      if (e)
        for (let t = 0; t < this.shaderPasses.length; t++)
          if (this.shaderPasses[t]._canDraw) {
            for (let s = 0; s < this.shaderPasses[t].textures.length; s++)
              if (!this.shaderPasses[t].textures[s]._canDraw) {
                e = !1
                break
              }
          } else {
            e = !1
            break
          }
      return e
    }
    contextRestored() {
      ;(this.getExtensions(),
        this.setBlendFunc(),
        this.setDepthFunc(),
        this.setDepthTest(!0),
        this.cache.clear(),
        this.scene.initStacks())
      for (let t = 0; t < this.renderTargets.length; t++) this.renderTargets[t]._restoreContext()
      for (let t = 0; t < this.planes.length; t++) this.planes[t]._restoreContext()
      for (let t = 0; t < this.shaderPasses.length; t++) this.shaderPasses[t]._restoreContext()
      const e = this.nextRender.add(() => {
        this.isContextexFullyRestored() &&
          ((e.keep = !1),
          (this.state.isContextLost = !1),
          this.onContextRestored && this.onContextRestored(),
          this.onSceneChange(),
          this.needRender())
      }, !0)
    }
    setPixelRatio(e) {
      this.pixelRatio = e
    }
    setSize() {
      if (!this.gl) return
      const e = this.container.getBoundingClientRect()
      ;((this._boundingRect = {
        width: e.width * this.pixelRatio,
        height: e.height * this.pixelRatio,
        top: e.top * this.pixelRatio,
        left: e.left * this.pixelRatio,
      }),
        (this.canvas.style.width = Math.floor(this._boundingRect.width / this.pixelRatio) + "px"),
        (this.canvas.style.height = Math.floor(this._boundingRect.height / this.pixelRatio) + "px"),
        (this.canvas.width = Math.floor(this._boundingRect.width * this._renderingScale)),
        (this.canvas.height = Math.floor(this._boundingRect.height * this._renderingScale)),
        this.gl.viewport(0, 0, this.gl.drawingBufferWidth, this.gl.drawingBufferHeight))
    }
    resize() {
      for (let e = 0; e < this.planes.length; e++) this.planes[e]._canDraw && this.planes[e].resize()
      for (let e = 0; e < this.shaderPasses.length; e++) this.shaderPasses[e]._canDraw && this.shaderPasses[e].resize()
      for (let e = 0; e < this.renderTargets.length; e++) this.renderTargets[e].resize()
      this.needRender()
    }
    clear() {
      this.gl.clear(this.gl.COLOR_BUFFER_BIT | this.gl.DEPTH_BUFFER_BIT)
    }
    clearDepth() {
      this.gl.clear(this.gl.DEPTH_BUFFER_BIT)
    }
    clearColor() {
      this.gl.clear(this.gl.COLOR_BUFFER_BIT)
    }
    bindFrameBuffer(e, t) {
      let s = null
      ;(e
        ? ((s = e.index),
          s !== this.state.frameBufferID &&
            (this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, e._frameBuffer),
            this.gl.viewport(0, 0, e._size.width, e._size.height),
            e._shouldClear && !t && this.clear()))
        : this.state.frameBufferID !== null &&
          (this.gl.bindFramebuffer(this.gl.FRAMEBUFFER, null),
          this.gl.viewport(0, 0, this.gl.drawingBufferWidth, this.gl.drawingBufferHeight)),
        (this.state.frameBufferID = s))
    }
    setDepthTest(e) {
      e && !this.state.depthTest
        ? ((this.state.depthTest = e), this.gl.enable(this.gl.DEPTH_TEST))
        : !e && this.state.depthTest && ((this.state.depthTest = e), this.gl.disable(this.gl.DEPTH_TEST))
    }
    setDepthFunc() {
      this.gl.depthFunc(this.gl.LEQUAL)
    }
    setBlending(e = !1) {
      e && !this.state.blending
        ? ((this.state.blending = e), this.gl.enable(this.gl.BLEND))
        : !e && this.state.blending && ((this.state.blending = e), this.gl.disable(this.gl.BLEND))
    }
    setBlendFunc() {
      ;(this.gl.enable(this.gl.BLEND),
        this.premultipliedAlpha
          ? this.gl.blendFuncSeparate(
              this.gl.ONE,
              this.gl.ONE_MINUS_SRC_ALPHA,
              this.gl.ONE,
              this.gl.ONE_MINUS_SRC_ALPHA,
            )
          : this.gl.blendFuncSeparate(
              this.gl.SRC_ALPHA,
              this.gl.ONE_MINUS_SRC_ALPHA,
              this.gl.ONE,
              this.gl.ONE_MINUS_SRC_ALPHA,
            ))
    }
    setFaceCulling(e) {
      if (this.state.cullFace !== e)
        if (((this.state.cullFace = e), e === "none")) this.gl.disable(this.gl.CULL_FACE)
        else {
          const t = e === "front" ? this.gl.FRONT : this.gl.BACK
          ;(this.gl.enable(this.gl.CULL_FACE), this.gl.cullFace(t))
        }
    }
    useProgram(e) {
      ;(this.state.currentProgramID === null || this.state.currentProgramID !== e.id) &&
        (this.gl.useProgram(e.program), (this.state.currentProgramID = e.id))
    }
    removePlane(e) {
      this.gl &&
        ((this.planes = this.planes.filter((t) => t.uuid !== e.uuid)),
        this.scene.removePlane(e),
        (e = null),
        this.gl && this.clear(),
        this.onSceneChange())
    }
    removeRenderTarget(e) {
      if (!this.gl) return
      let t = this.planes.find((s) => s.type !== "PingPongPlane" && s.target && s.target.uuid === e.uuid)
      for (let s = 0; s < this.planes.length; s++)
        this.planes[s].target && this.planes[s].target.uuid === e.uuid && (this.planes[s].target = null)
      this.renderTargets = this.renderTargets.filter((s) => s.uuid !== e.uuid)
      for (let s = 0; s < this.renderTargets.length; s++) this.renderTargets[s].index = s
      ;((e = null), this.gl && this.clear(), t && this.scene.resetPlaneStacks(), this.onSceneChange())
    }
    removeShaderPass(e) {
      this.gl &&
        ((this.shaderPasses = this.shaderPasses.filter((t) => t.uuid !== e.uuid)),
        this.scene.removeShaderPass(e),
        (e = null),
        this.gl && this.clear(),
        this.onSceneChange())
    }
    enableDrawing() {
      this.state.drawingEnabled = !0
    }
    disableDrawing() {
      this.state.drawingEnabled = !1
    }
    needRender() {
      this.state.forceRender = !0
    }
    render() {
      this.gl && (this.clear(), (this.state.currentGeometryID = null), this.scene.draw())
    }
    deletePrograms() {
      for (let e = 0; e < this.cache.programs.length; e++) {
        const t = this.cache.programs[e]
        this.gl.deleteProgram(t.program)
      }
    }
    dispose() {
      if (!this.gl) return
      for (this.state.isActive = !1; this.planes.length > 0; ) this.removePlane(this.planes[0])
      for (; this.shaderPasses.length > 0; ) this.removeShaderPass(this.shaderPasses[0])
      for (; this.renderTargets.length > 0; ) this.removeRenderTarget(this.renderTargets[0])
      let e = this.nextRender.add(() => {
        this.planes.length === 0 &&
          this.shaderPasses.length === 0 &&
          this.renderTargets.length === 0 &&
          ((e.keep = !1),
          this.deletePrograms(),
          this.clear(),
          this.canvas.removeEventListener("webgllost", this._contextLostHandler, !1),
          this.canvas.removeEventListener("webglrestored", this._contextRestoredHandler, !1),
          this.gl && this.extensions.WEBGL_lose_context && this.extensions.WEBGL_lose_context.loseContext(),
          (this.canvas.width = this.canvas.width),
          (this.gl = null),
          this.container.removeChild(this.canvas),
          (this.container = null),
          (this.canvas = null),
          this.onDisposed && this.onDisposed())
      }, !0)
    }
  }
  class rt {
    constructor({
      xOffset: e = 0,
      yOffset: t = 0,
      lastXDelta: s = 0,
      lastYDelta: i = 0,
      shouldWatch: r = !0,
      onScroll: a = () => {},
    } = {}) {
      ;((this.xOffset = e),
        (this.yOffset = t),
        (this.lastXDelta = s),
        (this.lastYDelta = i),
        (this.shouldWatch = r),
        (this.onScroll = a),
        (this.handler = this.scroll.bind(this, !0)),
        this.shouldWatch && window.addEventListener("scroll", this.handler, { passive: !0 }))
    }
    scroll() {
      this.updateScrollValues(window.pageXOffset, window.pageYOffset)
    }
    updateScrollValues(e, t) {
      const s = this.xOffset
      ;((this.xOffset = e), (this.lastXDelta = s - this.xOffset))
      const i = this.yOffset
      ;((this.yOffset = t),
        (this.lastYDelta = i - this.yOffset),
        this.onScroll && this.onScroll(this.lastXDelta, this.lastYDelta))
    }
    dispose() {
      this.shouldWatch && window.removeEventListener("scroll", this.handler, { passive: !0 })
    }
  }
  class at {
    constructor({
      container: e,
      alpha: t = !0,
      premultipliedAlpha: s = !1,
      antialias: i = !0,
      depth: r = !0,
      failIfMajorPerformanceCaveat: a = !0,
      preserveDrawingBuffer: h = !1,
      stencil: o = !1,
      autoResize: l = !0,
      autoRender: d = !0,
      watchScroll: u = !0,
      pixelRatio: c = window.devicePixelRatio || 1,
      renderingScale: f = 1,
      production: p = !1,
    } = {}) {
      ;((this.type = "Curtains"),
        (this._autoResize = l),
        (this._autoRender = d),
        (this._watchScroll = u),
        (this.pixelRatio = c),
        (f = isNaN(f) ? 1 : parseFloat(f)),
        (this._renderingScale = Math.max(0.25, Math.min(1, f))),
        (this.premultipliedAlpha = s),
        (this.alpha = t),
        (this.antialias = i),
        (this.depth = r),
        (this.failIfMajorPerformanceCaveat = a),
        (this.preserveDrawingBuffer = h),
        (this.stencil = o),
        (this.production = p),
        (this.errors = !1),
        e
          ? this.setContainer(e)
          : this.production ||
            g(
              this.type +
                ": no container provided in the initial parameters. Use setContainer() method to set one later and initialize the WebGL context",
            ))
    }
    setContainer(e) {
      if (e)
        if (typeof e == "string")
          if (((e = document.getElementById(e)), e)) this.container = e
          else {
            let t = document.createElement("div")
            ;(t.setAttribute("id", "curtains-canvas"),
              document.body.appendChild(t),
              (this.container = t),
              this.production ||
                g(
                  'Curtains: no valid container HTML element or ID provided, created a div with "curtains-canvas" ID instead',
                ))
          }
        else e instanceof Element && (this.container = e)
      else {
        let t = document.createElement("div")
        ;(t.setAttribute("id", "curtains-canvas"),
          document.body.appendChild(t),
          (this.container = t),
          this.production ||
            g(
              'Curtains: no valid container HTML element or ID provided, created a div with "curtains-canvas" ID instead',
            ))
      }
      this._initCurtains()
    }
    _initCurtains() {
      ;((this.planes = []),
        (this.renderTargets = []),
        (this.shaderPasses = []),
        this._initRenderer(),
        this.gl &&
          (this._initScroll(),
          this._setSize(),
          this._addListeners(),
          this.container.appendChild(this.canvas),
          (this._animationFrameID = null),
          this._autoRender && this._animate()))
    }
    _initRenderer() {
      ;((this.renderer = new it({
        alpha: this.alpha,
        antialias: this.antialias,
        premultipliedAlpha: this.premultipliedAlpha,
        depth: this.depth,
        failIfMajorPerformanceCaveat: this.failIfMajorPerformanceCaveat,
        preserveDrawingBuffer: this.preserveDrawingBuffer,
        stencil: this.stencil,
        container: this.container,
        pixelRatio: this.pixelRatio,
        renderingScale: this._renderingScale,
        production: this.production,
        onError: () => this._onRendererError(),
        onSuccess: () => this._onRendererSuccess(),
        onContextLost: () => this._onRendererContextLost(),
        onContextRestored: () => this._onRendererContextRestored(),
        onDisposed: () => this._onRendererDisposed(),
        onSceneChange: () => this._keepSync(),
      })),
        (this.gl = this.renderer.gl),
        (this.canvas = this.renderer.canvas))
    }
    restoreContext() {
      this.renderer.restoreContext()
    }
    _animate() {
      ;(this.render(), (this._animationFrameID = window.requestAnimationFrame(this._animate.bind(this))))
    }
    enableDrawing() {
      this.renderer.enableDrawing()
    }
    disableDrawing() {
      this.renderer.disableDrawing()
    }
    needRender() {
      this.renderer.needRender()
    }
    nextRender(e, t = !1) {
      return this.renderer.nextRender.add(e, t)
    }
    clear() {
      this.renderer && this.renderer.clear()
    }
    clearDepth() {
      this.renderer && this.renderer.clearDepth()
    }
    clearColor() {
      this.renderer && this.renderer.clearColor()
    }
    isWebGL2() {
      return this.gl ? this.renderer._isWebGL2 : !1
    }
    render() {
      ;(this.renderer.nextRender.execute(),
        !(!this.renderer.state.drawingEnabled && !this.renderer.state.forceRender) &&
          (this.renderer.state.forceRender && (this.renderer.state.forceRender = !1),
          this._onRenderCallback && this._onRenderCallback(),
          this.renderer.render()))
    }
    _addListeners() {
      ;((this._resizeHandler = null),
        this._autoResize &&
          ((this._resizeHandler = this.resize.bind(this, !0)),
          window.addEventListener("resize", this._resizeHandler, !1)))
    }
    setPixelRatio(e, t) {
      ;((this.pixelRatio = parseFloat(Math.max(e, 1)) || 1), this.renderer.setPixelRatio(e), this.resize(t))
    }
    _setSize() {
      ;(this.renderer.setSize(),
        this._scrollManager.shouldWatch &&
          ((this._scrollManager.xOffset = window.pageXOffset), (this._scrollManager.yOffset = window.pageYOffset)))
    }
    getBoundingRect() {
      return this.renderer._boundingRect
    }
    resize(e) {
      this.gl &&
        (this._setSize(),
        this.renderer.resize(),
        this.nextRender(() => {
          this._onAfterResizeCallback && e && this._onAfterResizeCallback()
        }))
    }
    _initScroll() {
      this._scrollManager = new rt({
        xOffset: window.pageXOffset,
        yOffset: 0,
        lastXDelta: 0,
        lastYDelta: 0,
        shouldWatch: this._watchScroll,
        onScroll: (e, t) => this._updateScroll(e, t),
      })
    }
    _updateScroll(e, t) {
      for (let s = 0; s < this.planes.length; s++)
        this.planes[s].watchScroll && this.planes[s].updateScrollPosition(e, t)
      ;(this.renderer.needRender(), this._onScrollCallback && this._onScrollCallback())
    }
    updateScrollValues(e, t) {
      this._scrollManager.updateScrollValues(e, t)
    }
    getScrollDeltas() {
      return { x: this._scrollManager.lastXDelta, y: this._scrollManager.lastYDelta }
    }
    getScrollValues() {
      return { x: this._scrollManager.xOffset, y: this._scrollManager.yOffset }
    }
    _keepSync() {
      ;((this.planes = this.renderer.planes),
        (this.shaderPasses = this.renderer.shaderPasses),
        (this.renderTargets = this.renderer.renderTargets))
    }
    lerp(e, t, s) {
      return Ke(e, t, s)
    }
    onAfterResize(e) {
      return (e && (this._onAfterResizeCallback = e), this)
    }
    onError(e) {
      return (e && (this._onErrorCallback = e), this)
    }
    _onRendererError() {
      setTimeout(() => {
        ;(this._onErrorCallback && !this.errors && this._onErrorCallback(), (this.errors = !0))
      }, 0)
    }
    onSuccess(e) {
      return (e && (this._onSuccessCallback = e), this)
    }
    _onRendererSuccess() {
      setTimeout(() => {
        this._onSuccessCallback && this._onSuccessCallback()
      }, 0)
    }
    onContextLost(e) {
      return (e && (this._onContextLostCallback = e), this)
    }
    _onRendererContextLost() {
      this._onContextLostCallback && this._onContextLostCallback()
    }
    onContextRestored(e) {
      return (e && (this._onContextRestoredCallback = e), this)
    }
    _onRendererContextRestored() {
      this._onContextRestoredCallback && this._onContextRestoredCallback()
    }
    onRender(e) {
      return (e && (this._onRenderCallback = e), this)
    }
    onScroll(e) {
      return (e && (this._onScrollCallback = e), this)
    }
    dispose() {
      this.renderer.dispose()
    }
    _onRendererDisposed() {
      ;(this._animationFrameID && window.cancelAnimationFrame(this._animationFrameID),
        this._resizeHandler && window.removeEventListener("resize", this._resizeHandler, !1),
        this._scrollManager && this._scrollManager.dispose())
    }
  }
  class nt {
    constructor(e, t, s) {
      if (((this.type = "Uniforms"), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        F(this.type + ": Renderer WebGL context is undefined", e)
        return
      }
      if (((this.renderer = e), (this.gl = e.gl), (this.program = t), (this.uniforms = {}), s))
        for (const i in s) {
          const r = s[i]
          this.uniforms[i] = {
            name: r.name,
            type: r.type,
            value: r.value.clone && typeof r.value.clone == "function" ? r.value.clone() : r.value,
            update: null,
          }
        }
    }
    handleUniformSetting(e) {
      switch (e.type) {
        case "1i":
          e.update = this.setUniform1i.bind(this)
          break
        case "1iv":
          e.update = this.setUniform1iv.bind(this)
          break
        case "1f":
          e.update = this.setUniform1f.bind(this)
          break
        case "1fv":
          e.update = this.setUniform1fv.bind(this)
          break
        case "2i":
          e.update = this.setUniform2i.bind(this)
          break
        case "2iv":
          e.update = this.setUniform2iv.bind(this)
          break
        case "2f":
          e.update = this.setUniform2f.bind(this)
          break
        case "2fv":
          e.update = this.setUniform2fv.bind(this)
          break
        case "3i":
          e.update = this.setUniform3i.bind(this)
          break
        case "3iv":
          e.update = this.setUniform3iv.bind(this)
          break
        case "3f":
          e.update = this.setUniform3f.bind(this)
          break
        case "3fv":
          e.update = this.setUniform3fv.bind(this)
          break
        case "4i":
          e.update = this.setUniform4i.bind(this)
          break
        case "4iv":
          e.update = this.setUniform4iv.bind(this)
          break
        case "4f":
          e.update = this.setUniform4f.bind(this)
          break
        case "4fv":
          e.update = this.setUniform4fv.bind(this)
          break
        case "mat2":
          e.update = this.setUniformMatrix2fv.bind(this)
          break
        case "mat3":
          e.update = this.setUniformMatrix3fv.bind(this)
          break
        case "mat4":
          e.update = this.setUniformMatrix4fv.bind(this)
          break
        default:
          this.renderer.production || g(this.type + ": This uniform type is not handled : ", e.type)
      }
    }
    setInternalFormat(e) {
      e.value.type === "Vec2"
        ? ((e._internalFormat = "Vec2"), (e.lastValue = e.value.clone()))
        : e.value.type === "Vec3"
          ? ((e._internalFormat = "Vec3"), (e.lastValue = e.value.clone()))
          : e.value.type === "Mat4"
            ? ((e._internalFormat = "Mat4"), (e.lastValue = e.value.clone()))
            : e.value.type === "Quat"
              ? ((e._internalFormat = "Quat"), (e.lastValue = e.value.clone()))
              : Array.isArray(e.value)
                ? ((e._internalFormat = "array"), (e.lastValue = Array.from(e.value)))
                : e.value.constructor === Float32Array
                  ? ((e._internalFormat = "mat"), (e.lastValue = e.value))
                  : ((e._internalFormat = "float"), (e.lastValue = e.value))
    }
    setUniforms() {
      if (this.uniforms)
        for (const e in this.uniforms) {
          let t = this.uniforms[e]
          ;((t.location = this.gl.getUniformLocation(this.program, t.name)),
            t._internalFormat || this.setInternalFormat(t),
            t.type ||
              (t._internalFormat === "Vec2"
                ? (t.type = "2f")
                : t._internalFormat === "Vec3"
                  ? (t.type = "3f")
                  : t._internalFormat === "Mat4"
                    ? (t.type = "mat4")
                    : t._internalFormat === "array"
                      ? t.value.length === 4
                        ? ((t.type = "4f"),
                          this.renderer.production ||
                            g(
                              this.type +
                                ": No uniform type declared for " +
                                t.name +
                                ", applied a 4f (array of 4 floats) uniform type",
                            ))
                        : t.value.length === 3
                          ? ((t.type = "3f"),
                            this.renderer.production ||
                              g(
                                this.type +
                                  ": No uniform type declared for " +
                                  t.name +
                                  ", applied a 3f (array of 3 floats) uniform type",
                              ))
                          : t.value.length === 2 &&
                            ((t.type = "2f"),
                            this.renderer.production ||
                              g(
                                this.type +
                                  ": No uniform type declared for " +
                                  t.name +
                                  ", applied a 2f (array of 2 floats) uniform type",
                              ))
                      : t._internalFormat === "mat"
                        ? t.value.length === 16
                          ? ((t.type = "mat4"),
                            this.renderer.production ||
                              g(
                                this.type +
                                  ": No uniform type declared for " +
                                  t.name +
                                  ", applied a mat4 (4x4 matrix array) uniform type",
                              ))
                          : t.value.length === 9
                            ? ((t.type = "mat3"),
                              this.renderer.production ||
                                g(
                                  this.type +
                                    ": No uniform type declared for " +
                                    t.name +
                                    ", applied a mat3 (3x3 matrix array) uniform type",
                                ))
                            : t.value.length === 4 &&
                              ((t.type = "mat2"),
                              this.renderer.production ||
                                g(
                                  this.type +
                                    ": No uniform type declared for " +
                                    t.name +
                                    ", applied a mat2 (2x2 matrix array) uniform type",
                                ))
                        : ((t.type = "1f"),
                          this.renderer.production ||
                            g(
                              this.type +
                                ": No uniform type declared for " +
                                t.name +
                                ", applied a 1f (float) uniform type",
                            ))),
            this.handleUniformSetting(t),
            t.update && t.update(t))
        }
    }
    updateUniforms() {
      if (this.uniforms)
        for (const e in this.uniforms) {
          const t = this.uniforms[e]
          let s = !1
          ;(t._internalFormat === "Vec2" || t._internalFormat === "Vec3" || t._internalFormat === "Quat"
            ? t.value.equals(t.lastValue) || ((s = !0), t.lastValue.copy(t.value))
            : t.value.length
              ? JSON.stringify(t.value) !== JSON.stringify(t.lastValue) &&
                ((s = !0), (t.lastValue = Array.from(t.value)))
              : t.value !== t.lastValue && ((s = !0), (t.lastValue = t.value)),
            s && t.update && t.update(t))
        }
    }
    setUniform1i(e) {
      this.gl.uniform1i(e.location, e.value)
    }
    setUniform1iv(e) {
      this.gl.uniform1iv(e.location, e.value)
    }
    setUniform1f(e) {
      this.gl.uniform1f(e.location, e.value)
    }
    setUniform1fv(e) {
      this.gl.uniform1fv(e.location, e.value)
    }
    setUniform2i(e) {
      e._internalFormat === "Vec2"
        ? this.gl.uniform2i(e.location, e.value.x, e.value.y)
        : this.gl.uniform2i(e.location, e.value[0], e.value[1])
    }
    setUniform2iv(e) {
      e._internalFormat === "Vec2"
        ? this.gl.uniform2iv(e.location, [e.value.x, e.value.y])
        : this.gl.uniform2iv(e.location, e.value)
    }
    setUniform2f(e) {
      e._internalFormat === "Vec2"
        ? this.gl.uniform2f(e.location, e.value.x, e.value.y)
        : this.gl.uniform2f(e.location, e.value[0], e.value[1])
    }
    setUniform2fv(e) {
      e._internalFormat === "Vec2"
        ? this.gl.uniform2fv(e.location, [e.value.x, e.value.y])
        : this.gl.uniform2fv(e.location, e.value)
    }
    setUniform3i(e) {
      e._internalFormat === "Vec3"
        ? this.gl.uniform3i(e.location, e.value.x, e.value.y, e.value.z)
        : this.gl.uniform3i(e.location, e.value[0], e.value[1], e.value[2])
    }
    setUniform3iv(e) {
      e._internalFormat === "Vec3"
        ? this.gl.uniform3iv(e.location, [e.value.x, e.value.y, e.value.z])
        : this.gl.uniform3iv(e.location, e.value)
    }
    setUniform3f(e) {
      e._internalFormat === "Vec3"
        ? this.gl.uniform3f(e.location, e.value.x, e.value.y, e.value.z)
        : this.gl.uniform3f(e.location, e.value[0], e.value[1], e.value[2])
    }
    setUniform3fv(e) {
      e._internalFormat === "Vec3"
        ? this.gl.uniform3fv(e.location, [e.value.x, e.value.y, e.value.z])
        : this.gl.uniform3fv(e.location, e.value)
    }
    setUniform4i(e) {
      e._internalFormat === "Quat"
        ? this.gl.uniform4i(e.location, e.value.elements[0], e.value.elements[1], e.value.elements[2], e.value[3])
        : this.gl.uniform4i(e.location, e.value[0], e.value[1], e.value[2], e.value[3])
    }
    setUniform4iv(e) {
      e._internalFormat === "Quat"
        ? this.gl.uniform4iv(e.location, [e.value.elements[0], e.value.elements[1], e.value.elements[2], e.value[3]])
        : this.gl.uniform4iv(e.location, e.value)
    }
    setUniform4f(e) {
      e._internalFormat === "Quat"
        ? this.gl.uniform4f(e.location, e.value.elements[0], e.value.elements[1], e.value.elements[2], e.value[3])
        : this.gl.uniform4f(e.location, e.value[0], e.value[1], e.value[2], e.value[3])
    }
    setUniform4fv(e) {
      e._internalFormat === "Quat"
        ? this.gl.uniform4fv(e.location, [e.value.elements[0], e.value.elements[1], e.value.elements[2], e.value[3]])
        : this.gl.uniform4fv(e.location, e.value)
    }
    setUniformMatrix2fv(e) {
      this.gl.uniformMatrix2fv(e.location, !1, e.value)
    }
    setUniformMatrix3fv(e) {
      this.gl.uniformMatrix3fv(e.location, !1, e.value)
    }
    setUniformMatrix4fv(e) {
      e._internalFormat === "Mat4"
        ? this.gl.uniformMatrix4fv(e.location, !1, e.value.elements)
        : this.gl.uniformMatrix4fv(e.location, !1, e.value)
    }
  }
  const ge = `
precision mediump float;
`.replace(/\n/g, ""),
    Oe = `
attribute vec3 aVertexPosition;
attribute vec2 aTextureCoord;
`.replace(/\n/g, ""),
    me = `
varying vec3 vVertexPosition;
varying vec2 vTextureCoord;
`.replace(/\n/g, ""),
    ht = (
      ge +
      Oe +
      me +
      `
uniform mat4 uMVMatrix;
uniform mat4 uPMatrix;

void main() {
    vTextureCoord = aTextureCoord;
    vVertexPosition = aVertexPosition;

    gl_Position = uPMatrix * uMVMatrix * vec4(aVertexPosition, 1.0);
}
`
    ).replace(/\n/g, ""),
    ot = (
      ge +
      me +
      `
void main() {
    gl_FragColor = vec4(0.0, 0.0, 0.0, 1.0);
}
`
    ).replace(/\n/g, ""),
    lt = (
      ge +
      Oe +
      me +
      `
void main() {
    vTextureCoord = aTextureCoord;
    vVertexPosition = aVertexPosition;

    gl_Position = vec4(aVertexPosition, 1.0);
}
`
    ).replace(/\n/g, ""),
    dt = (
      ge +
      me +
      `
uniform sampler2D uRenderTexture;

void main() {
    gl_FragColor = texture2D(uRenderTexture, vTextureCoord);
}
`
    ).replace(/\n/g, "")
  let Ue = 0
  class Ve {
    constructor(e, { parent: t, vertexShader: s, fragmentShader: i } = {}) {
      if (((this.type = "Program"), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        F(this.type + ": Renderer WebGL context is undefined", e)
        return
      }
      ;((this.renderer = e),
        (this.gl = this.renderer.gl),
        (this.parent = t),
        (this.defaultVsCode = this.parent.type === "Plane" ? ht : lt),
        (this.defaultFsCode = this.parent.type === "Plane" ? ot : dt),
        s
          ? (this.vsCode = s)
          : (!this.renderer.production &&
              this.parent.type === "Plane" &&
              g(this.parent.type + ": No vertex shader provided, will use a default one"),
            (this.vsCode = this.defaultVsCode)),
        i
          ? (this.fsCode = i)
          : (this.renderer.production || g(this.parent.type + ": No fragment shader provided, will use a default one"),
            (this.fsCode = this.defaultFsCode)),
        (this.compiled = !0),
        this.setupProgram())
    }
    createShader(e, t) {
      const s = this.gl.createShader(t)
      if (
        (this.gl.shaderSource(s, e),
        this.gl.compileShader(s),
        !this.renderer.production && !this.gl.getShaderParameter(s, this.gl.COMPILE_STATUS))
      ) {
        const i = t === this.gl.VERTEX_SHADER ? "vertex shader" : "fragment shader"
        let a = this.gl.getShaderSource(s).split(`
`)
        for (let h = 0; h < a.length; h++) a[h] = h + 1 + ": " + a[h]
        return (
          (a = a.join(`
`)),
          g(
            this.type + ": Errors occurred while compiling the",
            i,
            `:
`,
            this.gl.getShaderInfoLog(s),
          ),
          F(a),
          g(this.type + ": Will use a default", i),
          this.createShader(t === this.gl.VERTEX_SHADER ? this.defaultVsCode : this.defaultFsCode, t)
        )
      }
      return s
    }
    useNewShaders() {
      ;((this.vertexShader = this.createShader(this.vsCode, this.gl.VERTEX_SHADER)),
        (this.fragmentShader = this.createShader(this.fsCode, this.gl.FRAGMENT_SHADER)),
        (!this.vertexShader || !this.fragmentShader) &&
          (this.renderer.production || g(this.type + ": Unable to find or compile the vertex or fragment shader")))
    }
    setupProgram() {
      let e = this.renderer.cache.getProgramFromShaders(this.vsCode, this.fsCode)
      e
        ? ((this.vertexShader = e.vertexShader),
          (this.fragmentShader = e.fragmentShader),
          (this.activeUniforms = e.activeUniforms),
          (this.activeAttributes = e.activeAttributes),
          this.createProgram())
        : (this.useNewShaders(), this.compiled && (this.createProgram(), this.renderer.cache.addProgram(this)))
    }
    createProgram() {
      if (
        (Ue++,
        (this.id = Ue),
        (this.program = this.gl.createProgram()),
        this.gl.attachShader(this.program, this.vertexShader),
        this.gl.attachShader(this.program, this.fragmentShader),
        this.gl.linkProgram(this.program),
        !this.renderer.production && !this.gl.getProgramParameter(this.program, this.gl.LINK_STATUS))
      ) {
        ;(g(this.type + ": Unable to initialize the shader program: " + this.gl.getProgramInfoLog(this.program)),
          g(this.type + ": Will use default vertex and fragment shaders"),
          (this.vertexShader = this.createShader(this.defaultVsCode, this.gl.VERTEX_SHADER)),
          (this.fragmentShader = this.createShader(this.defaultFsCode, this.gl.FRAGMENT_SHADER)),
          this.createProgram())
        return
      }
      if (
        (this.gl.deleteShader(this.vertexShader),
        this.gl.deleteShader(this.fragmentShader),
        !this.activeUniforms || !this.activeAttributes)
      ) {
        this.activeUniforms = { textures: [], textureMatrices: [] }
        const e = this.gl.getProgramParameter(this.program, this.gl.ACTIVE_UNIFORMS)
        for (let s = 0; s < e; s++) {
          const i = this.gl.getActiveUniform(this.program, s)
          ;(i.type === this.gl.SAMPLER_2D && this.activeUniforms.textures.push(i.name),
            i.type === this.gl.FLOAT_MAT4 &&
              i.name !== "uMVMatrix" &&
              i.name !== "uPMatrix" &&
              this.activeUniforms.textureMatrices.push(i.name))
        }
        this.activeAttributes = []
        const t = this.gl.getProgramParameter(this.program, this.gl.ACTIVE_ATTRIBUTES)
        for (let s = 0; s < t; s++) {
          const i = this.gl.getActiveAttrib(this.program, s)
          this.activeAttributes.push(i.name)
        }
      }
    }
    createUniforms(e) {
      ;((this.uniformsManager = new nt(this.renderer, this.program, e)), this.setUniforms())
    }
    setUniforms() {
      ;(this.renderer.useProgram(this), this.uniformsManager.setUniforms())
    }
    updateUniforms() {
      ;(this.renderer.useProgram(this), this.uniformsManager.updateUniforms())
    }
  }
  class ct {
    constructor(e, { program: t = null, width: s = 1, height: i = 1 } = {}) {
      if (((this.type = "Geometry"), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        F(this.type + ": Renderer WebGL context is undefined", e)
        return
      }
      ;((this.renderer = e),
        (this.gl = this.renderer.gl),
        (this.definition = { id: s * i + s, width: s, height: i }),
        this.setDefaultAttributes(),
        this.setVerticesUVs())
    }
    restoreContext(e) {
      ;((this.program = null), this.setDefaultAttributes(), this.setVerticesUVs(), this.setProgram(e))
    }
    setDefaultAttributes() {
      this.attributes = {
        vertexPosition: { name: "aVertexPosition", size: 3, isActive: !1 },
        textureCoord: { name: "aTextureCoord", size: 3, isActive: !1 },
      }
    }
    setVerticesUVs() {
      const e = this.renderer.cache.getGeometryFromID(this.definition.id)
      e
        ? ((this.attributes.vertexPosition.array = e.vertices), (this.attributes.textureCoord.array = e.uvs))
        : (this.computeVerticesUVs(),
          this.renderer.cache.addGeometry(
            this.definition.id,
            this.attributes.vertexPosition.array,
            this.attributes.textureCoord.array,
          ))
    }
    setProgram(e) {
      ;((this.program = e),
        this.initAttributes(),
        this.renderer._isWebGL2
          ? ((this._vao = this.gl.createVertexArray()), this.gl.bindVertexArray(this._vao))
          : this.renderer.extensions.OES_vertex_array_object &&
            ((this._vao = this.renderer.extensions.OES_vertex_array_object.createVertexArrayOES()),
            this.renderer.extensions.OES_vertex_array_object.bindVertexArrayOES(this._vao)),
        this.initializeBuffers())
    }
    initAttributes() {
      for (const e in this.attributes) {
        if (
          ((this.attributes[e].isActive = this.program.activeAttributes.includes(this.attributes[e].name)),
          !this.attributes[e].isActive)
        )
          return
        ;((this.attributes[e].location = this.gl.getAttribLocation(this.program.program, this.attributes[e].name)),
          (this.attributes[e].buffer = this.gl.createBuffer()),
          (this.attributes[e].numberOfItems =
            this.definition.width * this.definition.height * this.attributes[e].size * 2))
      }
    }
    computeVerticesUVs() {
      ;((this.attributes.vertexPosition.array = []), (this.attributes.textureCoord.array = []))
      const e = this.attributes.vertexPosition.array,
        t = this.attributes.textureCoord.array
      for (let s = 0; s < this.definition.height; s++) {
        const i = s / this.definition.height
        for (let r = 0; r < this.definition.width; r++) {
          const a = r / this.definition.width
          ;(t.push(a),
            t.push(i),
            t.push(0),
            e.push((a - 0.5) * 2),
            e.push((i - 0.5) * 2),
            e.push(0),
            t.push(a + 1 / this.definition.width),
            t.push(i),
            t.push(0),
            e.push((a + 1 / this.definition.width - 0.5) * 2),
            e.push((i - 0.5) * 2),
            e.push(0),
            t.push(a),
            t.push(i + 1 / this.definition.height),
            t.push(0),
            e.push((a - 0.5) * 2),
            e.push((i + 1 / this.definition.height - 0.5) * 2),
            e.push(0),
            t.push(a),
            t.push(i + 1 / this.definition.height),
            t.push(0),
            e.push((a - 0.5) * 2),
            e.push((i + 1 / this.definition.height - 0.5) * 2),
            e.push(0),
            t.push(a + 1 / this.definition.width),
            t.push(i),
            t.push(0),
            e.push((a + 1 / this.definition.width - 0.5) * 2),
            e.push((i - 0.5) * 2),
            e.push(0),
            t.push(a + 1 / this.definition.width),
            t.push(i + 1 / this.definition.height),
            t.push(0),
            e.push((a + 1 / this.definition.width - 0.5) * 2),
            e.push((i + 1 / this.definition.height - 0.5) * 2),
            e.push(0))
        }
      }
    }
    initializeBuffers() {
      if (this.attributes) {
        for (const e in this.attributes) {
          if (!this.attributes[e].isActive) return
          ;(this.gl.enableVertexAttribArray(this.attributes[e].location),
            this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.attributes[e].buffer),
            this.gl.bufferData(this.gl.ARRAY_BUFFER, new Float32Array(this.attributes[e].array), this.gl.STATIC_DRAW),
            this.gl.vertexAttribPointer(this.attributes[e].location, this.attributes[e].size, this.gl.FLOAT, !1, 0, 0))
        }
        this.renderer.state.currentGeometryID = this.definition.id
      }
    }
    bindBuffers() {
      if (this._vao)
        this.renderer._isWebGL2
          ? this.gl.bindVertexArray(this._vao)
          : this.renderer.extensions.OES_vertex_array_object.bindVertexArrayOES(this._vao)
      else
        for (const e in this.attributes) {
          if (!this.attributes[e].isActive) return
          ;(this.gl.enableVertexAttribArray(this.attributes[e].location),
            this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.attributes[e].buffer),
            this.gl.vertexAttribPointer(this.attributes[e].location, this.attributes[e].size, this.gl.FLOAT, !1, 0, 0))
        }
      this.renderer.state.currentGeometryID = this.definition.id
    }
    draw() {
      this.gl.drawArrays(this.gl.TRIANGLES, 0, this.attributes.vertexPosition.numberOfItems)
    }
    dispose() {
      this._vao &&
        (this.renderer._isWebGL2
          ? this.gl.deleteVertexArray(this._vao)
          : this.renderer.extensions.OES_vertex_array_object.deleteVertexArrayOES(this._vao))
      for (const e in this.attributes) {
        if (!this.attributes[e].isActive) return
        ;(this.gl.bindBuffer(this.gl.ARRAY_BUFFER, this.attributes[e].buffer),
          this.gl.bufferData(this.gl.ARRAY_BUFFER, 1, this.gl.STATIC_DRAW),
          this.gl.deleteBuffer(this.attributes[e].buffer))
      }
      ;((this.attributes = null), (this.renderer.state.currentGeometryID = null))
    }
  }
  class Z {
    constructor(e = new Float32Array([1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1])) {
      ;((this.type = "Mat4"), (this.elements = e))
    }
    setFromArray(e) {
      for (let t = 0; t < this.elements.length; t++) this.elements[t] = e[t]
      return this
    }
    copy(e) {
      const t = e.elements
      return (
        (this.elements[0] = t[0]),
        (this.elements[1] = t[1]),
        (this.elements[2] = t[2]),
        (this.elements[3] = t[3]),
        (this.elements[4] = t[4]),
        (this.elements[5] = t[5]),
        (this.elements[6] = t[6]),
        (this.elements[7] = t[7]),
        (this.elements[8] = t[8]),
        (this.elements[9] = t[9]),
        (this.elements[10] = t[10]),
        (this.elements[11] = t[11]),
        (this.elements[12] = t[12]),
        (this.elements[13] = t[13]),
        (this.elements[14] = t[14]),
        (this.elements[15] = t[15]),
        this
      )
    }
    clone() {
      return new Z().copy(this)
    }
    multiply(e) {
      const t = this.elements,
        s = e.elements
      let i = new Z()
      return (
        (i.elements[0] = s[0] * t[0] + s[1] * t[4] + s[2] * t[8] + s[3] * t[12]),
        (i.elements[1] = s[0] * t[1] + s[1] * t[5] + s[2] * t[9] + s[3] * t[13]),
        (i.elements[2] = s[0] * t[2] + s[1] * t[6] + s[2] * t[10] + s[3] * t[14]),
        (i.elements[3] = s[0] * t[3] + s[1] * t[7] + s[2] * t[11] + s[3] * t[15]),
        (i.elements[4] = s[4] * t[0] + s[5] * t[4] + s[6] * t[8] + s[7] * t[12]),
        (i.elements[5] = s[4] * t[1] + s[5] * t[5] + s[6] * t[9] + s[7] * t[13]),
        (i.elements[6] = s[4] * t[2] + s[5] * t[6] + s[6] * t[10] + s[7] * t[14]),
        (i.elements[7] = s[4] * t[3] + s[5] * t[7] + s[6] * t[11] + s[7] * t[15]),
        (i.elements[8] = s[8] * t[0] + s[9] * t[4] + s[10] * t[8] + s[11] * t[12]),
        (i.elements[9] = s[8] * t[1] + s[9] * t[5] + s[10] * t[9] + s[11] * t[13]),
        (i.elements[10] = s[8] * t[2] + s[9] * t[6] + s[10] * t[10] + s[11] * t[14]),
        (i.elements[11] = s[8] * t[3] + s[9] * t[7] + s[10] * t[11] + s[11] * t[15]),
        (i.elements[12] = s[12] * t[0] + s[13] * t[4] + s[14] * t[8] + s[15] * t[12]),
        (i.elements[13] = s[12] * t[1] + s[13] * t[5] + s[14] * t[9] + s[15] * t[13]),
        (i.elements[14] = s[12] * t[2] + s[13] * t[6] + s[14] * t[10] + s[15] * t[14]),
        (i.elements[15] = s[12] * t[3] + s[13] * t[7] + s[14] * t[11] + s[15] * t[15]),
        i
      )
    }
    getInverse() {
      const e = this.elements,
        t = new Z(),
        s = t.elements
      let i = e[0],
        r = e[1],
        a = e[2],
        h = e[3],
        o = e[4],
        l = e[5],
        d = e[6],
        u = e[7],
        c = e[8],
        f = e[9],
        p = e[10],
        m = e[11],
        T = e[12],
        S = e[13],
        R = e[14],
        E = e[15],
        M = i * l - r * o,
        k = i * d - a * o,
        C = i * u - h * o,
        y = r * d - a * l,
        O = r * u - h * l,
        X = a * u - h * d,
        U = c * S - f * T,
        V = c * R - p * T,
        q = c * E - m * T,
        P = f * R - p * S,
        w = f * E - m * S,
        v = p * E - m * R,
        x = M * v - k * w + C * P + y * q - O * V + X * U
      return x
        ? ((x = 1 / x),
          (s[0] = (l * v - d * w + u * P) * x),
          (s[1] = (a * w - r * v - h * P) * x),
          (s[2] = (S * X - R * O + E * y) * x),
          (s[3] = (p * O - f * X - m * y) * x),
          (s[4] = (d * q - o * v - u * V) * x),
          (s[5] = (i * v - a * q + h * V) * x),
          (s[6] = (R * C - T * X - E * k) * x),
          (s[7] = (c * X - p * C + m * k) * x),
          (s[8] = (o * w - l * q + u * U) * x),
          (s[9] = (r * q - i * w - h * U) * x),
          (s[10] = (T * O - S * C + E * M) * x),
          (s[11] = (f * C - c * O - m * M) * x),
          (s[12] = (l * V - o * P - d * U) * x),
          (s[13] = (i * P - r * V + a * U) * x),
          (s[14] = (S * k - T * y - R * M) * x),
          (s[15] = (c * y - f * k + p * M) * x),
          t)
        : null
    }
    scale(e) {
      let t = this.elements
      return (
        (t[0] *= e.x),
        (t[1] *= e.x),
        (t[2] *= e.x),
        (t[3] *= e.x),
        (t[4] *= e.y),
        (t[5] *= e.y),
        (t[6] *= e.y),
        (t[7] *= e.y),
        (t[8] *= e.z),
        (t[9] *= e.z),
        (t[10] *= e.z),
        (t[11] *= e.z),
        this
      )
    }
    compose(e, t, s) {
      let i = this.elements
      const r = t.elements[0],
        a = t.elements[1],
        h = t.elements[2],
        o = t.elements[3],
        l = r + r,
        d = a + a,
        u = h + h,
        c = r * l,
        f = r * d,
        p = r * u,
        m = a * d,
        T = a * u,
        S = h * u,
        R = o * l,
        E = o * d,
        M = o * u,
        k = s.x,
        C = s.y,
        y = s.z
      return (
        (i[0] = (1 - (m + S)) * k),
        (i[1] = (f + M) * k),
        (i[2] = (p - E) * k),
        (i[3] = 0),
        (i[4] = (f - M) * C),
        (i[5] = (1 - (c + S)) * C),
        (i[6] = (T + R) * C),
        (i[7] = 0),
        (i[8] = (p + E) * y),
        (i[9] = (T - R) * y),
        (i[10] = (1 - (c + m)) * y),
        (i[11] = 0),
        (i[12] = e.x),
        (i[13] = e.y),
        (i[14] = e.z),
        (i[15] = 1),
        this
      )
    }
    composeFromOrigin(e, t, s, i) {
      let r = this.elements
      const a = t.elements[0],
        h = t.elements[1],
        o = t.elements[2],
        l = t.elements[3],
        d = a + a,
        u = h + h,
        c = o + o,
        f = a * d,
        p = a * u,
        m = a * c,
        T = h * u,
        S = h * c,
        R = o * c,
        E = l * d,
        M = l * u,
        k = l * c,
        C = s.x,
        y = s.y,
        O = s.z,
        X = i.x,
        U = i.y,
        V = i.z,
        q = (1 - (T + R)) * C,
        P = (p + k) * C,
        w = (m - M) * C,
        v = (p - k) * y,
        x = (1 - (f + R)) * y,
        z = (S + E) * y,
        Y = (m + M) * O,
        ie = (S - E) * O,
        B = (1 - (f + T)) * O
      return (
        (r[0] = q),
        (r[1] = P),
        (r[2] = w),
        (r[3] = 0),
        (r[4] = v),
        (r[5] = x),
        (r[6] = z),
        (r[7] = 0),
        (r[8] = Y),
        (r[9] = ie),
        (r[10] = B),
        (r[11] = 0),
        (r[12] = e.x + X - (q * X + v * U + Y * V)),
        (r[13] = e.y + U - (P * X + x * U + ie * V)),
        (r[14] = e.z + V - (w * X + z * U + B * V)),
        (r[15] = 1),
        this
      )
    }
  }
  class A {
    constructor(e = 0, t = e) {
      ;((this.type = "Vec2"), (this._x = e), (this._y = t))
    }
    get x() {
      return this._x
    }
    get y() {
      return this._y
    }
    set x(e) {
      const t = e !== this._x
      ;((this._x = e), t && this._onChangeCallback && this._onChangeCallback())
    }
    set y(e) {
      const t = e !== this._y
      ;((this._y = e), t && this._onChangeCallback && this._onChangeCallback())
    }
    onChange(e) {
      return (e && (this._onChangeCallback = e), this)
    }
    set(e, t) {
      return ((this._x = e), (this._y = t), this)
    }
    add(e) {
      return ((this._x += e.x), (this._y += e.y), this)
    }
    addScalar(e) {
      return ((this._x += e), (this._y += e), this)
    }
    sub(e) {
      return ((this._x -= e.x), (this._y -= e.y), this)
    }
    subScalar(e) {
      return ((this._x -= e), (this._y -= e), this)
    }
    multiply(e) {
      return ((this._x *= e.x), (this._y *= e.y), this)
    }
    multiplyScalar(e) {
      return ((this._x *= e), (this._y *= e), this)
    }
    copy(e) {
      return ((this._x = e.x), (this._y = e.y), this)
    }
    clone() {
      return new A(this._x, this._y)
    }
    sanitizeNaNValuesWith(e) {
      return (
        (this._x = isNaN(this._x) ? e.x : parseFloat(this._x)),
        (this._y = isNaN(this._y) ? e.y : parseFloat(this._y)),
        this
      )
    }
    max(e) {
      return ((this._x = Math.max(this._x, e.x)), (this._y = Math.max(this._y, e.y)), this)
    }
    min(e) {
      return ((this._x = Math.min(this._x, e.x)), (this._y = Math.min(this._y, e.y)), this)
    }
    equals(e) {
      return this._x === e.x && this._y === e.y
    }
    normalize() {
      let e = this._x * this._x + this._y * this._y
      return (e > 0 && (e = 1 / Math.sqrt(e)), (this._x *= e), (this._y *= e), this)
    }
    dot(e) {
      return this._x * e.x + this._y * e.y
    }
  }
  class b {
    constructor(e = 0, t = e, s = e) {
      ;((this.type = "Vec3"), (this._x = e), (this._y = t), (this._z = s))
    }
    get x() {
      return this._x
    }
    get y() {
      return this._y
    }
    get z() {
      return this._z
    }
    set x(e) {
      const t = e !== this._x
      ;((this._x = e), t && this._onChangeCallback && this._onChangeCallback())
    }
    set y(e) {
      const t = e !== this._y
      ;((this._y = e), t && this._onChangeCallback && this._onChangeCallback())
    }
    set z(e) {
      const t = e !== this._z
      ;((this._z = e), t && this._onChangeCallback && this._onChangeCallback())
    }
    onChange(e) {
      return (e && (this._onChangeCallback = e), this)
    }
    set(e, t, s) {
      return ((this._x = e), (this._y = t), (this._z = s), this)
    }
    add(e) {
      return ((this._x += e.x), (this._y += e.y), (this._z += e.z), this)
    }
    addScalar(e) {
      return ((this._x += e), (this._y += e), (this._z += e), this)
    }
    sub(e) {
      return ((this._x -= e.x), (this._y -= e.y), (this._z -= e.z), this)
    }
    subScalar(e) {
      return ((this._x -= e), (this._y -= e), (this._z -= e), this)
    }
    multiply(e) {
      return ((this._x *= e.x), (this._y *= e.y), (this._z *= e.z), this)
    }
    multiplyScalar(e) {
      return ((this._x *= e), (this._y *= e), (this._z *= e), this)
    }
    copy(e) {
      return ((this._x = e.x), (this._y = e.y), (this._z = e.z), this)
    }
    clone() {
      return new b(this._x, this._y, this._z)
    }
    sanitizeNaNValuesWith(e) {
      return (
        (this._x = isNaN(this._x) ? e.x : parseFloat(this._x)),
        (this._y = isNaN(this._y) ? e.y : parseFloat(this._y)),
        (this._z = isNaN(this._z) ? e.z : parseFloat(this._z)),
        this
      )
    }
    max(e) {
      return (
        (this._x = Math.max(this._x, e.x)),
        (this._y = Math.max(this._y, e.y)),
        (this._z = Math.max(this._z, e.z)),
        this
      )
    }
    min(e) {
      return (
        (this._x = Math.min(this._x, e.x)),
        (this._y = Math.min(this._y, e.y)),
        (this._z = Math.min(this._z, e.z)),
        this
      )
    }
    equals(e) {
      return this._x === e.x && this._y === e.y && this._z === e.z
    }
    normalize() {
      let e = this._x * this._x + this._y * this._y + this._z * this._z
      return (e > 0 && (e = 1 / Math.sqrt(e)), (this._x *= e), (this._y *= e), (this._z *= e), this)
    }
    dot(e) {
      return this._x * e.x + this._y * e.y + this._z * e.z
    }
    applyMat4(e) {
      const t = this._x,
        s = this._y,
        i = this._z,
        r = e.elements
      let a = r[3] * t + r[7] * s + r[11] * i + r[15]
      return (
        (a = a || 1),
        (this._x = (r[0] * t + r[4] * s + r[8] * i + r[12]) / a),
        (this._y = (r[1] * t + r[5] * s + r[9] * i + r[13]) / a),
        (this._z = (r[2] * t + r[6] * s + r[10] * i + r[14]) / a),
        this
      )
    }
    applyQuat(e) {
      const t = this._x,
        s = this._y,
        i = this._z,
        r = e.elements[0],
        a = e.elements[1],
        h = e.elements[2],
        o = e.elements[3],
        l = o * t + a * i - h * s,
        d = o * s + h * t - r * i,
        u = o * i + r * s - a * t,
        c = -r * t - a * s - h * i
      return (
        (this._x = l * o + c * -r + d * -h - u * -a),
        (this._y = d * o + c * -a + u * -r - l * -h),
        (this._z = u * o + c * -h + l * -a - d * -r),
        this
      )
    }
    project(e) {
      return (this.applyMat4(e.viewMatrix).applyMat4(e.projectionMatrix), this)
    }
    unproject(e) {
      return (this.applyMat4(e.projectionMatrix.getInverse()).applyMat4(e.worldMatrix), this)
    }
  }
  const Ee = new A(),
    ut = new b(),
    ft = new Z()
  class ae {
    constructor(
      e,
      {
        isFBOTexture: t = !1,
        fromTexture: s = !1,
        loader: i,
        sampler: r,
        floatingPoint: a = "none",
        premultiplyAlpha: h = !1,
        anisotropy: o = 1,
        generateMipmap: l = null,
        wrapS: d,
        wrapT: u,
        minFilter: c,
        magFilter: f,
      } = {},
    ) {
      if (((this.type = "Texture"), (e = (e && e.renderer) || e), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        e.production ||
          F(this.type + ": Unable to create a " + this.type + " because the Renderer WebGL context is not defined")
        return
      }
      if (
        ((this.renderer = e),
        (this.gl = this.renderer.gl),
        (this.uuid = Se()),
        (this._globalParameters = {
          unpackAlignment: 4,
          flipY: !t,
          premultiplyAlpha: !1,
          shouldPremultiplyAlpha: h,
          floatingPoint: a,
          type: this.gl.UNSIGNED_BYTE,
          internalFormat: this.gl.RGBA,
          format: this.gl.RGBA,
        }),
        (this.parameters = {
          anisotropy: o,
          generateMipmap: l,
          wrapS: d || this.gl.CLAMP_TO_EDGE,
          wrapT: u || this.gl.CLAMP_TO_EDGE,
          minFilter: c || this.gl.LINEAR,
          magFilter: f || this.gl.LINEAR,
          _shouldUpdate: !0,
        }),
        this._initState(),
        (this.sourceType = t ? "fbo" : "empty"),
        (this._useCache = !0),
        (this._samplerName = r),
        (this._sampler = { isActive: !1, isTextureBound: !1, texture: this.gl.createTexture() }),
        (this._textureMatrix = { matrix: new Z(), isActive: !1 }),
        (this._size = { width: 1, height: 1 }),
        (this.scale = new A(1)),
        this.scale.onChange(() => this.resize()),
        (this.offset = new A()),
        this.offset.onChange(() => this.resize()),
        (this._loader = i),
        (this._sourceLoaded = !1),
        (this._uploaded = !1),
        (this._willUpdate = !1),
        (this.shouldUpdate = !1),
        (this._forceUpdate = !1),
        (this.userData = {}),
        (this._canDraw = !1),
        s)
      ) {
        ;((this._copyOnInit = !0), (this._copiedFrom = s))
        return
      }
      ;((this._copyOnInit = !1), this._initTexture())
    }
    _initState() {
      this._state = {
        anisotropy: 1,
        generateMipmap: !1,
        wrapS: null,
        wrapT: null,
        minFilter: null,
        magFilter: this.gl.LINEAR,
      }
    }
    _initTexture() {
      ;(this.gl.bindTexture(this.gl.TEXTURE_2D, this._sampler.texture),
        this.sourceType === "empty" &&
          ((this._globalParameters.flipY = !1),
          this._updateGlobalTexParameters(),
          this.gl.texImage2D(
            this.gl.TEXTURE_2D,
            0,
            this.gl.RGBA,
            1,
            1,
            0,
            this.gl.RGBA,
            this.gl.UNSIGNED_BYTE,
            new Uint8Array([0, 0, 0, 255]),
          ),
          (this._canDraw = !0)))
    }
    _restoreFromTexture() {
      ;(this._copyOnInit || this._initTexture(),
        this._parent && (this._setTextureUniforms(), this._setSize()),
        this.copy(this._copiedFrom),
        (this._canDraw = !0))
    }
    _restoreContext() {
      if (
        ((this._canDraw = !1),
        (this._sampler.texture = this.gl.createTexture()),
        (this._sampler.isActive = !1),
        (this._sampler.isTextureBound = !1),
        (this._textureMatrix.isActive = !1),
        this._initState(),
        (this._state.generateMipmap = !1),
        (this.parameters._shouldUpdate = !0),
        !this._copiedFrom)
      )
        (this._initTexture(),
          this._parent && this._setParent(),
          this.source &&
            (this.setSource(this.source),
            this.sourceType === "image" ? this.renderer.cache.addTexture(this) : this.needUpdate()),
          (this._canDraw = !0))
      else {
        const e = this.renderer.nextRender.add(() => {
          this._copiedFrom._canDraw && (this._restoreFromTexture(), (e.keep = !1))
        }, !0)
      }
    }
    addParent(e) {
      if (
        !e ||
        (e.type !== "Plane" && e.type !== "PingPongPlane" && e.type !== "ShaderPass" && e.type !== "RenderTarget")
      ) {
        this.renderer.production ||
          g(this.type + ": cannot add texture as a child of ", e, " because it is not a valid parent")
        return
      }
      ;((this._parent = e),
        (this.index = this._parent.textures.length),
        this._parent.textures.push(this),
        this._setParent())
    }
    _setParent() {
      if (
        ((this._sampler.name = this._samplerName || "uSampler" + this.index),
        (this._textureMatrix.name = this._samplerName ? this._samplerName + "Matrix" : "uTextureMatrix" + this.index),
        this._parent._program)
      ) {
        if (!this._parent._program.compiled) {
          this.renderer.production || g(this.type + ": Unable to create the texture because the program is not valid")
          return
        }
        if ((this._setTextureUniforms(), this._copyOnInit)) {
          const e = this.renderer.nextRender.add(() => {
            this._copiedFrom._canDraw && this._copiedFrom._uploaded && (this.copy(this._copiedFrom), (e.keep = !1))
          }, !0)
          return
        }
        ;(this.source
          ? this._parent.loader && this._parent.loader._addSourceToParent(this.source, this.sourceType)
          : (this._size = {
              width: this._parent._boundingRect.document.width,
              height: this._parent._boundingRect.document.height,
            }),
          this._setSize())
      } else
        this._parent.type === "RenderTarget" &&
          ((this._size = {
            width: (this._parent._size && this._parent._size.width) || this.renderer._boundingRect.width,
            height: (this._parent._size && this._parent._size.height) || this.renderer._boundingRect.height,
          }),
          this._upload(),
          this._updateTexParameters(),
          (this._canDraw = !0))
    }
    hasParent() {
      return !!this._parent
    }
    _setTextureUniforms() {
      const e = this._parent._program.activeUniforms
      for (let t = 0; t < e.textures.length; t++)
        e.textures[t] === this._sampler.name &&
          ((this._sampler.isActive = !0),
          this.renderer.useProgram(this._parent._program),
          (this._sampler.location = this.gl.getUniformLocation(this._parent._program.program, this._sampler.name)),
          e.textureMatrices.find((i) => i === this._textureMatrix.name) &&
            ((this._textureMatrix.isActive = !0),
            (this._textureMatrix.location = this.gl.getUniformLocation(
              this._parent._program.program,
              this._textureMatrix.name,
            ))),
          this.gl.uniform1i(this._sampler.location, this.index))
    }
    copy(e) {
      if (!e || e.type !== "Texture") {
        this.renderer.production || g(this.type + ": Unable to set the texture from texture:", e)
        return
      }
      ;((this._globalParameters = Object.assign({}, e._globalParameters)),
        (this._state = Object.assign({}, e._state)),
        (this.parameters.generateMipmap = e.parameters.generateMipmap),
        (this._state.generateMipmap = null),
        (this._size = e._size),
        !this._sourceLoaded && e._sourceLoaded && this._onSourceLoadedCallback && this._onSourceLoadedCallback(),
        (this._sourceLoaded = e._sourceLoaded),
        !this._uploaded && e._uploaded && this._onSourceUploadedCallback && this._onSourceUploadedCallback(),
        (this._uploaded = e._uploaded),
        (this.sourceType = e.sourceType),
        (this.source = e.source),
        (this._videoFrameCallbackID = e._videoFrameCallbackID),
        (this._sampler.texture = e._sampler.texture),
        (this._copiedFrom = e),
        this._parent &&
          this._parent._program &&
          (!this._canDraw || !this._textureMatrix.matrix) &&
          (this._setSize(), (this._canDraw = !0)),
        this._updateTexParameters(),
        this.renderer.needRender())
    }
    setSource(e) {
      this._sourceLoaded ||
        this.renderer.nextRender.add(() => this._onSourceLoadedCallback && this._onSourceLoadedCallback())
      const t = e.tagName.toUpperCase() === "IMG" ? "image" : e.tagName.toLowerCase()
      if (((t === "video" || t === "canvas") && (this._useCache = !1), this._useCache)) {
        const s = this.renderer.cache.getTextureFromSource(e)
        if (s && s.uuid !== this.uuid) {
          ;(this._uploaded ||
            (this.renderer.nextRender.add(() => this._onSourceUploadedCallback && this._onSourceUploadedCallback()),
            (this._uploaded = !0)),
            this.copy(s),
            this.resize())
          return
        }
      }
      if (this.sourceType === "empty" || this.sourceType !== t)
        if (t === "video") ((this._willUpdate = !1), (this.shouldUpdate = !0))
        else if (t === "canvas") ((this._willUpdate = !0), (this.shouldUpdate = !0))
        else if (t === "image") ((this._willUpdate = !1), (this.shouldUpdate = !1))
        else {
          this.renderer.production || g(this.type + ": this HTML tag could not be converted into a texture:", e.tagName)
          return
        }
      ;((this.source = e),
        (this.sourceType = t),
        (this._size = {
          width: this.source.naturalWidth || this.source.width || this.source.videoWidth,
          height: this.source.naturalHeight || this.source.height || this.source.videoHeight,
        }),
        (this._sourceLoaded = !0),
        this.gl.bindTexture(this.gl.TEXTURE_2D, this._sampler.texture),
        this.resize(),
        (this._globalParameters.flipY = !0),
        (this._globalParameters.premultiplyAlpha = this._globalParameters.shouldPremultiplyAlpha),
        this.sourceType === "image" &&
          ((this.parameters.generateMipmap = this.parameters.generateMipmap || this.parameters.generateMipmap === null),
          (this.parameters._shouldUpdate = this.parameters.generateMipmap),
          (this._state.generateMipmap = !1),
          this._upload()),
        this.renderer.needRender())
    }
    _updateGlobalTexParameters() {
      ;(this.renderer.state.unpackAlignment !== this._globalParameters.unpackAlignment &&
        (this.gl.pixelStorei(this.gl.UNPACK_ALIGNMENT, this._globalParameters.unpackAlignment),
        (this.renderer.state.unpackAlignment = this._globalParameters.unpackAlignment)),
        this.renderer.state.flipY !== this._globalParameters.flipY &&
          (this.gl.pixelStorei(this.gl.UNPACK_FLIP_Y_WEBGL, this._globalParameters.flipY),
          (this.renderer.state.flipY = this._globalParameters.flipY)),
        this.renderer.state.premultiplyAlpha !== this._globalParameters.premultiplyAlpha &&
          (this.gl.pixelStorei(this.gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, this._globalParameters.premultiplyAlpha),
          (this.renderer.state.premultiplyAlpha = this._globalParameters.premultiplyAlpha)),
        this._globalParameters.floatingPoint === "half-float"
          ? this.renderer._isWebGL2 && this.renderer.extensions.EXT_color_buffer_float
            ? ((this._globalParameters.internalFormat = this.gl.RGBA16F),
              (this._globalParameters.type = this.gl.HALF_FLOAT))
            : this.renderer.extensions.OES_texture_half_float
              ? (this._globalParameters.type = this.renderer.extensions.OES_texture_half_float.HALF_FLOAT_OES)
              : this.renderer.production ||
                g(this.type + ": could not use half-float textures because the extension is not available")
          : this._globalParameters.floatingPoint === "float" &&
            (this.renderer._isWebGL2 && this.renderer.extensions.EXT_color_buffer_float
              ? ((this._globalParameters.internalFormat = this.gl.RGBA16F),
                (this._globalParameters.type = this.gl.FLOAT))
              : this.renderer.extensions.OES_texture_float
                ? (this._globalParameters.type = this.renderer.extensions.OES_texture_half_float.FLOAT)
                : this.renderer.production ||
                  g(this.type + ": could not use float textures because the extension is not available")))
    }
    _updateTexParameters() {
      ;(this.index && this.renderer.state.activeTexture !== this.index && this._bindTexture(),
        this.parameters.wrapS !== this._state.wrapS &&
          (!this.renderer._isWebGL2 &&
            (!Q(this._size.width) || !Q(this._size.height)) &&
            (this.parameters.wrapS = this.gl.CLAMP_TO_EDGE),
          this.parameters.wrapS !== this.gl.REPEAT &&
            this.parameters.wrapS !== this.gl.CLAMP_TO_EDGE &&
            this.parameters.wrapS !== this.gl.MIRRORED_REPEAT &&
            (this.renderer.production ||
              g(
                this.type + ": Wrong wrapS value",
                this.parameters.wrapS,
                "for this texture:",
                this,
                `
gl.CLAMP_TO_EDGE wrapping will be used instead`,
              ),
            (this.parameters.wrapS = this.gl.CLAMP_TO_EDGE)),
          this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_S, this.parameters.wrapS),
          (this._state.wrapS = this.parameters.wrapS)),
        this.parameters.wrapT !== this._state.wrapT &&
          (!this.renderer._isWebGL2 &&
            (!Q(this._size.width) || !Q(this._size.height)) &&
            (this.parameters.wrapT = this.gl.CLAMP_TO_EDGE),
          this.parameters.wrapT !== this.gl.REPEAT &&
            this.parameters.wrapT !== this.gl.CLAMP_TO_EDGE &&
            this.parameters.wrapT !== this.gl.MIRRORED_REPEAT &&
            (this.renderer.production ||
              g(
                this.type + ": Wrong wrapT value",
                this.parameters.wrapT,
                "for this texture:",
                this,
                `
gl.CLAMP_TO_EDGE wrapping will be used instead`,
              ),
            (this.parameters.wrapT = this.gl.CLAMP_TO_EDGE)),
          this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_WRAP_T, this.parameters.wrapT),
          (this._state.wrapT = this.parameters.wrapT)),
        this.parameters.generateMipmap &&
          !this._state.generateMipmap &&
          this.source &&
          (!this.renderer._isWebGL2 && (!Q(this._size.width) || !Q(this._size.height))
            ? (this.parameters.generateMipmap = !1)
            : this.gl.generateMipmap(this.gl.TEXTURE_2D),
          (this._state.generateMipmap = this.parameters.generateMipmap)),
        this.parameters.minFilter !== this._state.minFilter &&
          (!this.renderer._isWebGL2 &&
            (!Q(this._size.width) || !Q(this._size.height)) &&
            (this.parameters.minFilter = this.gl.LINEAR),
          !this.parameters.generateMipmap &&
            this.parameters.generateMipmap !== null &&
            (this.parameters.minFilter = this.gl.LINEAR),
          this.parameters.minFilter !== this.gl.LINEAR &&
            this.parameters.minFilter !== this.gl.NEAREST &&
            this.parameters.minFilter !== this.gl.NEAREST_MIPMAP_NEAREST &&
            this.parameters.minFilter !== this.gl.LINEAR_MIPMAP_NEAREST &&
            this.parameters.minFilter !== this.gl.NEAREST_MIPMAP_LINEAR &&
            this.parameters.minFilter !== this.gl.LINEAR_MIPMAP_LINEAR &&
            (this.renderer.production ||
              g(
                this.type + ": Wrong minFilter value",
                this.parameters.minFilter,
                "for this texture:",
                this,
                `
gl.LINEAR filtering will be used instead`,
              ),
            (this.parameters.minFilter = this.gl.LINEAR)),
          this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MIN_FILTER, this.parameters.minFilter),
          (this._state.minFilter = this.parameters.minFilter)),
        this.parameters.magFilter !== this._state.magFilter &&
          (!this.renderer._isWebGL2 &&
            (!Q(this._size.width) || !Q(this._size.height)) &&
            (this.parameters.magFilter = this.gl.LINEAR),
          this.parameters.magFilter !== this.gl.LINEAR &&
            this.parameters.magFilter !== this.gl.NEAREST &&
            (this.renderer.production ||
              g(
                this.type + ": Wrong magFilter value",
                this.parameters.magFilter,
                "for this texture:",
                this,
                `
gl.LINEAR filtering will be used instead`,
              ),
            (this.parameters.magFilter = this.gl.LINEAR)),
          this.gl.texParameteri(this.gl.TEXTURE_2D, this.gl.TEXTURE_MAG_FILTER, this.parameters.magFilter),
          (this._state.magFilter = this.parameters.magFilter)))
      const e = this.renderer.extensions.EXT_texture_filter_anisotropic
      if (e && this.parameters.anisotropy !== this._state.anisotropy) {
        const t = this.gl.getParameter(e.MAX_TEXTURE_MAX_ANISOTROPY_EXT)
        ;((this.parameters.anisotropy = Math.max(1, Math.min(this.parameters.anisotropy, t))),
          this.gl.texParameterf(this.gl.TEXTURE_2D, e.TEXTURE_MAX_ANISOTROPY_EXT, this.parameters.anisotropy),
          (this._state.anisotropy = this.parameters.anisotropy))
      }
    }
    setWrapS(e) {
      this.parameters.wrapS !== e && ((this.parameters.wrapS = e), (this.parameters._shouldUpdate = !0))
    }
    setWrapT(e) {
      this.parameters.wrapT !== e && ((this.parameters.wrapT = e), (this.parameters._shouldUpdate = !0))
    }
    setMinFilter(e) {
      this.parameters.minFilter !== e && ((this.parameters.minFilter = e), (this.parameters._shouldUpdate = !0))
    }
    setMagFilter(e) {
      this.parameters.magFilter !== e && ((this.parameters.magFilter = e), (this.parameters._shouldUpdate = !0))
    }
    setAnisotropy(e) {
      ;((e = isNaN(e) ? this.parameters.anisotropy : e),
        this.parameters.anisotropy !== e && ((this.parameters.anisotropy = e), (this.parameters._shouldUpdate = !0)))
    }
    needUpdate() {
      this._forceUpdate = !0
    }
    _videoFrameCallback() {
      if (((this._willUpdate = !0), this.source))
        this.source.requestVideoFrameCallback(() => this._videoFrameCallback())
      else {
        const e = this.renderer.nextRender.add(() => {
          this.source && ((e.keep = !1), this.source.requestVideoFrameCallback(() => this._videoFrameCallback()))
        }, !0)
      }
    }
    _upload() {
      ;(this._updateGlobalTexParameters(),
        this.source
          ? this.gl.texImage2D(
              this.gl.TEXTURE_2D,
              0,
              this._globalParameters.internalFormat,
              this._globalParameters.format,
              this._globalParameters.type,
              this.source,
            )
          : this.sourceType === "fbo" &&
            this.gl.texImage2D(
              this.gl.TEXTURE_2D,
              0,
              this._globalParameters.internalFormat,
              this._size.width,
              this._size.height,
              0,
              this._globalParameters.format,
              this._globalParameters.type,
              this.source || null,
            ),
        this._uploaded ||
          (this.renderer.nextRender.add(() => this._onSourceUploadedCallback && this._onSourceUploadedCallback()),
          (this._uploaded = !0)))
    }
    _getSizes() {
      if (this.sourceType === "fbo")
        return {
          parentWidth: this._parent._boundingRect.document.width,
          parentHeight: this._parent._boundingRect.document.height,
          sourceWidth: this._parent._boundingRect.document.width,
          sourceHeight: this._parent._boundingRect.document.height,
          xOffset: 0,
          yOffset: 0,
        }
      const e = this._parent.scale ? Ee.set(this._parent.scale.x, this._parent.scale.y) : Ee.set(1, 1),
        t = this._parent._boundingRect.document.width * e.x,
        s = this._parent._boundingRect.document.height * e.y,
        i = this._size.width,
        r = this._size.height,
        a = i / r,
        h = t / s
      let o = 0,
        l = 0
      return (
        h > a ? (l = Math.min(0, s - t * (1 / a))) : h < a && (o = Math.min(0, t - s * a)),
        { parentWidth: t, parentHeight: s, sourceWidth: i, sourceHeight: r, xOffset: o, yOffset: l }
      )
    }
    setScale(e) {
      if (!e.type || e.type !== "Vec2") {
        this.renderer.production ||
          g(this.type + ": Cannot set scale because the parameter passed is not of Vec2 type:", e)
        return
      }
      ;(e.sanitizeNaNValuesWith(this.scale).max(Ee.set(0.001, 0.001)),
        e.equals(this.scale) || (this.scale.copy(e), this.resize()))
    }
    setOffset(e) {
      if (!e.type || e.type !== "Vec2") {
        this.renderer.production ||
          g(this.type + ": Cannot set offset because the parameter passed is not of Vec2 type:", scale)
        return
      }
      ;(e.sanitizeNaNValuesWith(this.offset), e.equals(this.offset) || (this.offset.copy(e), this.resize()))
    }
    _setSize() {
      if (this._parent && this._parent._program) {
        const e = this._getSizes()
        this._updateTextureMatrix(e)
      }
    }
    resize() {
      ;(this.sourceType === "fbo"
        ? ((this._size = {
            width: (this._parent._size && this._parent._size.width) || this._parent._boundingRect.document.width,
            height: (this._parent._size && this._parent._size.height) || this._parent._boundingRect.document.height,
          }),
          this._copiedFrom ||
            (this.gl.bindTexture(this.gl.TEXTURE_2D, this._sampler.texture),
            this.gl.texImage2D(
              this.gl.TEXTURE_2D,
              0,
              this._globalParameters.internalFormat,
              this._size.width,
              this._size.height,
              0,
              this._globalParameters.format,
              this._globalParameters.type,
              null,
            )))
        : this.source &&
          (this._size = {
            width: this.source.naturalWidth || this.source.width || this.source.videoWidth,
            height: this.source.naturalHeight || this.source.height || this.source.videoHeight,
          }),
        this._setSize())
    }
    _updateTextureMatrix(e) {
      const t = ut.set(e.parentWidth / (e.parentWidth - e.xOffset), e.parentHeight / (e.parentHeight - e.yOffset), 1)
      ;((t.x /= this.scale.x),
        (t.y /= this.scale.y),
        (this._textureMatrix.matrix = ft.setFromArray([
          t.x,
          0,
          0,
          0,
          0,
          t.y,
          0,
          0,
          0,
          0,
          1,
          0,
          (1 - t.x) / 2 + this.offset.x,
          (1 - t.y) / 2 + this.offset.y,
          0,
          1,
        ])),
        this._updateMatrixUniform())
    }
    _updateMatrixUniform() {
      this._textureMatrix.isActive &&
        (this.renderer.useProgram(this._parent._program),
        this.gl.uniformMatrix4fv(this._textureMatrix.location, !1, this._textureMatrix.matrix.elements))
    }
    _onSourceLoaded(e) {
      ;(this.setSource(e), this.sourceType === "image" && this.renderer.cache.addTexture(this))
    }
    _bindTexture() {
      this._canDraw &&
        (this.renderer.state.activeTexture !== this.index &&
          (this.gl.activeTexture(this.gl.TEXTURE0 + this.index), (this.renderer.state.activeTexture = this.index)),
        this.gl.bindTexture(this.gl.TEXTURE_2D, this._sampler.texture),
        this._sampler.isTextureBound ||
          ((this._sampler.isTextureBound = !!this.gl.getParameter(this.gl.TEXTURE_BINDING_2D)),
          this._sampler.isTextureBound && this.renderer.needRender()))
    }
    _draw() {
      ;(this._sampler.isActive &&
        (this._bindTexture(),
        this.sourceType === "video" &&
          this.source &&
          !this._videoFrameCallbackID &&
          this.source.readyState >= this.source.HAVE_CURRENT_DATA &&
          !this.source.paused &&
          (this._willUpdate = !0),
        (this._forceUpdate || (this._willUpdate && this.shouldUpdate)) &&
          ((this._state.generateMipmap = !1), this._upload()),
        this.sourceType === "video" && (this._willUpdate = !1),
        (this._forceUpdate = !1)),
        this.parameters._shouldUpdate && (this._updateTexParameters(), (this.parameters._shouldUpdate = !1)))
    }
    onSourceLoaded(e) {
      return (e && (this._onSourceLoadedCallback = e), this)
    }
    onSourceUploaded(e) {
      return (e && (this._onSourceUploadedCallback = e), this)
    }
    _dispose(e = !1) {
      var s
      ;(this.sourceType === "video" || (this.sourceType === "image" && !this.renderer.state.isActive)
        ? (this._loader && this._loader._removeSource(this), (this.source = null))
        : this.sourceType === "canvas" &&
          this.source &&
          ((this.source.width = (s = this.source) == null ? void 0 : s.width), (this.source = null)),
        (this._parent = null),
        this.gl &&
          !this._copiedFrom &&
          (e || this.sourceType !== "image" || !this.renderer.state.isActive) &&
          ((this._canDraw = !1),
          this.renderer.cache.removeTexture(this),
          this.gl.activeTexture(this.gl.TEXTURE0 + this.index),
          this.gl.bindTexture(this.gl.TEXTURE_2D, null),
          this.gl.deleteTexture(this._sampler.texture)))
    }
  }
  class Be {
    constructor(e, t = "anonymous") {
      if (((this.type = "TextureLoader"), (e = (e && e.renderer) || e), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        F(this.type + ": Renderer WebGL context is undefined", e)
        return
      }
      ;((this.renderer = e), (this.gl = this.renderer.gl), (this.crossOrigin = t), (this.elements = []))
    }
    _addElement(e, t, s, i) {
      const r = {
        source: e,
        texture: t,
        load: this._sourceLoaded.bind(this, e, t, s),
        error: this._sourceLoadError.bind(this, e, i),
      }
      return (this.elements.push(r), r)
    }
    _sourceLoadError(e, t, s) {
      t && t(e, s)
    }
    _sourceLoaded(e, t, s) {
      t._sourceLoaded ||
        (t._onSourceLoaded(e),
        this._parent &&
          (this._increment && this._increment(),
          this.renderer.nextRender.add(() => this._parent._onLoadingCallback && this._parent._onLoadingCallback(t))),
        s && s(t))
    }
    _getSourceType(e) {
      let t
      return (
        typeof e == "string"
          ? e.match(/\.(jpeg|jpg|jfif|pjpeg|pjp|gif|bmp|png|webp|svg|avif|apng)$/) !== null
            ? (t = "image")
            : e.match(/\.(webm|mp4|mpg|mpeg|avi|ogg|ogm|ogv|mov|av1)$/) !== null && (t = "video")
          : e.tagName.toUpperCase() === "IMG"
            ? (t = "image")
            : e.tagName.toUpperCase() === "VIDEO"
              ? (t = "video")
              : e.tagName.toUpperCase() === "CANVAS" && (t = "canvas"),
        t
      )
    }
    _createImage(e) {
      if (typeof e == "string" || !e.hasAttribute("crossOrigin")) {
        const t = new Image()
        return (
          (t.crossOrigin = this.crossOrigin),
          typeof e == "string"
            ? (t.src = e)
            : ((t.src = e.src),
              e.hasAttribute("data-sampler") && t.setAttribute("data-sampler", e.getAttribute("data-sampler"))),
          t
        )
      } else return e
    }
    _createVideo(e) {
      if (typeof e == "string" || e.getAttribute("crossOrigin") === null) {
        const t = document.createElement("video")
        return (
          (t.crossOrigin = this.crossOrigin),
          typeof e == "string"
            ? (t.src = e)
            : ((t.src = e.src),
              e.hasAttribute("data-sampler") && t.setAttribute("data-sampler", e.getAttribute("data-sampler"))),
          t
        )
      } else return e
    }
    loadSource(e, t, s, i) {
      switch (this._getSourceType(e)) {
        case "image":
          this.loadImage(e, t, s, i)
          break
        case "video":
          this.loadVideo(e, t, s, i)
          break
        case "canvas":
          this.loadCanvas(e, t, s)
          break
        default:
          this._sourceLoadError(e, i, "this source could not be converted into a texture: " + e)
          break
      }
    }
    loadSources(e, t, s, i) {
      for (let r = 0; r < e.length; r++) this.loadSource(e[r], t, s, i)
    }
    loadImage(e, t = {}, s, i) {
      const r = this.renderer.cache.getTextureFromSource(e)
      let a = Object.assign({}, t)
      if ((this._parent && (a = Object.assign(a, this._parent._texturesOptions)), (a.loader = this), r)) {
        ;((a.sampler =
          typeof e != "string" && e.hasAttribute("data-sampler") ? e.getAttribute("data-sampler") : a.sampler),
          (a.fromTexture = r))
        const d = new ae(this.renderer, a)
        ;(this._sourceLoaded(r.source, d, s), this._parent && this._addToParent(d, r.source, "image"))
        return
      }
      const h = this._createImage(e)
      a.sampler = h.hasAttribute("data-sampler") ? h.getAttribute("data-sampler") : a.sampler
      const o = new ae(this.renderer, a),
        l = this._addElement(h, o, s, i)
      ;(h.complete
        ? this._sourceLoaded(h, o, s)
        : h.decode
          ? h
              .decode()
              .then(this._sourceLoaded.bind(this, h, o, s))
              .catch(() => {
                ;(h.addEventListener("load", l.load, !1), h.addEventListener("error", l.error, !1))
              })
          : (h.addEventListener("load", l.load, !1), h.addEventListener("error", l.error, !1)),
        this._parent && this._addToParent(o, h, "image"))
    }
    loadImages(e, t, s, i) {
      for (let r = 0; r < e.length; r++) this.loadImage(e[r], t, s, i)
    }
    loadVideo(e, t = {}, s, i) {
      const r = this._createVideo(e)
      ;((r.preload = !0),
        (r.muted = !0),
        (r.loop = !0),
        r.setAttribute("playsinline", ""),
        (r.crossOrigin = this.crossOrigin))
      let a = Object.assign({}, t)
      ;(this._parent && (a = Object.assign(t, this._parent._texturesOptions)),
        (a.loader = this),
        (a.sampler = r.hasAttribute("data-sampler") ? r.getAttribute("data-sampler") : a.sampler))
      const h = new ae(this.renderer, a),
        o = this._addElement(r, h, s, i)
      ;(r.addEventListener("canplaythrough", o.load, !1),
        r.addEventListener("error", o.error, !1),
        r.readyState >= r.HAVE_FUTURE_DATA && s && this._sourceLoaded(r, h, s),
        r.load(),
        this._addToParent && this._addToParent(h, r, "video"),
        "requestVideoFrameCallback" in HTMLVideoElement.prototype &&
          ((o.videoFrameCallback = h._videoFrameCallback.bind(h)),
          (h._videoFrameCallbackID = r.requestVideoFrameCallback(o.videoFrameCallback))))
    }
    loadVideos(e, t, s, i) {
      for (let r = 0; r < e.length; r++) this.loadVideo(e[r], t, s, i)
    }
    loadCanvas(e, t = {}, s) {
      let i = Object.assign({}, t)
      ;(this._parent && (i = Object.assign(t, this._parent._texturesOptions)),
        (i.loader = this),
        (i.sampler = e.hasAttribute("data-sampler") ? e.getAttribute("data-sampler") : i.sampler))
      const r = new ae(this.renderer, i)
      ;(this._addElement(e, r, s, null), this._sourceLoaded(e, r, s), this._parent && this._addToParent(r, e, "canvas"))
    }
    loadCanvases(e, t, s) {
      for (let i = 0; i < e.length; i++) this.loadCanvas(e[i], t, s)
    }
    _removeSource(e) {
      const t = this.elements.find((s) => s.texture.uuid === e.uuid)
      t &&
        (e.sourceType === "image"
          ? t.source.removeEventListener("load", t.load, !1)
          : e.sourceType === "video" &&
            (t.videoFrameCallback &&
              e._videoFrameCallbackID &&
              t.source.cancelVideoFrameCallback(e._videoFrameCallbackID),
            t.source.removeEventListener("canplaythrough", t.load, !1),
            t.source.pause(),
            t.source.removeAttribute("src"),
            t.source.load()),
        t.source.removeEventListener("error", t.error, !1))
    }
  }
  class pt extends Be {
    constructor(e, t, { sourcesLoaded: s = 0, sourcesToLoad: i = 0, complete: r = !1, onComplete: a = () => {} } = {}) {
      ;(super(e, t.crossOrigin),
        (this.type = "PlaneTextureLoader"),
        (this._parent = t),
        this._parent.type !== "Plane" &&
          this._parent.type !== "PingPongPlane" &&
          this._parent.type !== "ShaderPass" &&
          (g(this.type + ": Wrong parent type assigned to this loader"), (this._parent = null)),
        (this.sourcesLoaded = s),
        (this.sourcesToLoad = i),
        (this.complete = r),
        (this.onComplete = a))
    }
    _setLoaderSize(e) {
      ;((this.sourcesToLoad = e),
        this.sourcesToLoad === 0 &&
          ((this.complete = !0), this.renderer.nextRender.add(() => this.onComplete && this.onComplete())))
    }
    _increment() {
      ;(this.sourcesLoaded++,
        this.sourcesLoaded >= this.sourcesToLoad &&
          !this.complete &&
          ((this.complete = !0), this.renderer.nextRender.add(() => this.onComplete && this.onComplete())))
    }
    _addSourceToParent(e, t) {
      if (t === "image") {
        const s = this._parent.images
        !s.find((r) => r.src === e.src) && s.push(e)
      } else if (t === "video") {
        const s = this._parent.videos
        !s.find((r) => r.src === e.src) && s.push(e)
      } else if (t === "canvas") {
        const s = this._parent.canvases
        !s.find((r) => r.isSameNode(e)) && s.push(e)
      }
    }
    _addToParent(e, t, s) {
      ;(this._addSourceToParent(t, s), this._parent && e.addParent(this._parent))
    }
  }
  class gt {
    constructor(
      e,
      t = "Mesh",
      {
        vertexShaderID: s,
        fragmentShaderID: i,
        vertexShader: r,
        fragmentShader: a,
        uniforms: h = {},
        widthSegments: o = 1,
        heightSegments: l = 1,
        renderOrder: d = 0,
        depthTest: u = !0,
        cullFace: c = "back",
        texturesOptions: f = {},
        crossOrigin: p = "anonymous",
      } = {},
    ) {
      if (
        ((this.type = t),
        (e = (e && e.renderer) || e),
        (!e || e.type !== "Renderer") &&
          (F(this.type + ": Curtains not passed as first argument or Curtains Renderer is missing", e),
          setTimeout(() => {
            this._onErrorCallback && this._onErrorCallback()
          }, 0)),
        (this.renderer = e),
        (this.gl = this.renderer.gl),
        !this.gl)
      ) {
        ;(this.renderer.production ||
          F(this.type + ": Unable to create a " + this.type + " because the Renderer WebGL context is not defined"),
          setTimeout(() => {
            this._onErrorCallback && this._onErrorCallback()
          }, 0))
        return
      }
      ;((this._canDraw = !1),
        (this.renderOrder = d),
        (this._depthTest = u),
        (this.cullFace = c),
        this.cullFace !== "back" && this.cullFace !== "front" && this.cullFace !== "none" && (this.cullFace = "back"),
        (this.textures = []),
        (this._texturesOptions = Object.assign(
          {
            premultiplyAlpha: !1,
            anisotropy: 1,
            floatingPoint: "none",
            wrapS: this.gl.CLAMP_TO_EDGE,
            wrapT: this.gl.CLAMP_TO_EDGE,
            minFilter: this.gl.LINEAR,
            magFilter: this.gl.LINEAR,
          },
          f,
        )),
        (this.crossOrigin = p),
        !r && s && document.getElementById(s) && (r = document.getElementById(s).innerHTML),
        !a && i && document.getElementById(i) && (a = document.getElementById(i).innerHTML),
        this._initMesh(),
        (o = parseInt(o)),
        (l = parseInt(l)),
        (this._geometry = new ct(this.renderer, { width: o, height: l })),
        (this._program = new Ve(this.renderer, { parent: this, vertexShader: r, fragmentShader: a })),
        this._program.compiled
          ? (this._program.createUniforms(h),
            (this.uniforms = this._program.uniformsManager.uniforms),
            this._geometry.setProgram(this._program),
            this.renderer.onSceneChange())
          : this.renderer.nextRender.add(() => this._onErrorCallback && this._onErrorCallback()))
    }
    _initMesh() {
      ;((this.uuid = Se()),
        (this.loader = new pt(this.renderer, this, {
          sourcesLoaded: 0,
          initSourcesToLoad: 0,
          complete: !1,
          onComplete: () => {
            ;(this._onReadyCallback && this._onReadyCallback(), this.renderer.needRender())
          },
        })),
        (this.images = []),
        (this.videos = []),
        (this.canvases = []),
        (this.userData = {}),
        (this._canDraw = !0))
    }
    _restoreContext() {
      ;((this._canDraw = !1),
        this._matrices && (this._matrices = null),
        (this._program = new Ve(this.renderer, {
          parent: this,
          vertexShader: this._program.vsCode,
          fragmentShader: this._program.fsCode,
        })),
        this._program.compiled &&
          (this._geometry.restoreContext(this._program),
          this._program.createUniforms(this.uniforms),
          (this.uniforms = this._program.uniformsManager.uniforms),
          this._programRestored()))
    }
    setRenderTarget(e) {
      if (!e || e.type !== "RenderTarget") {
        this.renderer.production ||
          g(
            this.type +
              ": Could not set the render target because the argument passed is not a RenderTarget class object",
            e,
          )
        return
      }
      ;(this.type === "Plane" && this.renderer.scene.removePlane(this),
        (this.target = e),
        this.type === "Plane" && this.renderer.scene.addPlane(this))
    }
    setRenderOrder(e = 0) {
      ;((e = isNaN(e) ? this.renderOrder : parseInt(e)),
        e !== this.renderOrder && ((this.renderOrder = e), this.renderer.scene.setPlaneRenderOrder(this)))
    }
    createTexture(e = {}) {
      const t = new ae(this.renderer, Object.assign(e, this._texturesOptions))
      return (t.addParent(this), t)
    }
    addTexture(e) {
      if (!e || e.type !== "Texture") {
        this.renderer.production ||
          g(this.type + ": cannot add ", e, " to this " + this.type + " because it is not a valid texture")
        return
      }
      e.addParent(this)
    }
    loadSources(e, t = {}, s, i) {
      for (let r = 0; r < e.length; r++) this.loadSource(e[r], t, s, i)
    }
    loadSource(e, t = {}, s, i) {
      this.loader.loadSource(
        e,
        Object.assign(t, this._texturesOptions),
        (r) => {
          s && s(r)
        },
        (r, a) => {
          ;(this.renderer.production ||
            g(this.type + ": this HTML tag could not be converted into a texture:", r.tagName),
            i && i(r, a))
        },
      )
    }
    loadImage(e, t = {}, s, i) {
      this.loader.loadImage(
        e,
        Object.assign(t, this._texturesOptions),
        (r) => {
          s && s(r)
        },
        (r, a) => {
          ;(this.renderer.production ||
            g(
              this.type +
                `: There has been an error:
`,
              a,
              `
while loading this image:
`,
              r,
            ),
            i && i(r, a))
        },
      )
    }
    loadVideo(e, t = {}, s, i) {
      this.loader.loadVideo(
        e,
        Object.assign(t, this._texturesOptions),
        (r) => {
          s && s(r)
        },
        (r, a) => {
          ;(this.renderer.production ||
            g(
              this.type +
                `: There has been an error:
`,
              a,
              `
while loading this video:
`,
              r,
            ),
            i && i(r, a))
        },
      )
    }
    loadCanvas(e, t = {}, s) {
      this.loader.loadCanvas(e, Object.assign(t, this._texturesOptions), (i) => {
        s && s(i)
      })
    }
    loadImages(e, t = {}, s, i) {
      for (let r = 0; r < e.length; r++) this.loadImage(e[r], t, s, i)
    }
    loadVideos(e, t = {}, s, i) {
      for (let r = 0; r < e.length; r++) this.loadVideo(e[r], t, s, i)
    }
    loadCanvases(e, t = {}, s) {
      for (let i = 0; i < e.length; i++) this.loadCanvas(e[i], t, s)
    }
    playVideos() {
      for (let e = 0; e < this.textures.length; e++) {
        const t = this.textures[e]
        if (t.sourceType === "video") {
          const s = t.source.play()
          s !== void 0 &&
            s.catch((i) => {
              this.renderer.production || g(this.type + ": Could not play the video : ", i)
            })
        }
      }
    }
    _draw() {
      ;(this.renderer.setDepthTest(this._depthTest),
        this.renderer.setFaceCulling(this.cullFace),
        this._program.updateUniforms(),
        this._geometry.bindBuffers(),
        (this.renderer.state.forceBufferUpdate = !1))
      for (let e = 0; e < this.textures.length; e++)
        if ((this.textures[e]._draw(), this.textures[e]._sampler.isActive && !this.textures[e]._sampler.isTextureBound))
          return
      ;(this._geometry.draw(),
        (this.renderer.state.activeTexture = null),
        this._onAfterRenderCallback && this._onAfterRenderCallback())
    }
    onError(e) {
      return (e && (this._onErrorCallback = e), this)
    }
    onLoading(e) {
      return (e && (this._onLoadingCallback = e), this)
    }
    onReady(e) {
      return (e && (this._onReadyCallback = e), this)
    }
    onRender(e) {
      return (e && (this._onRenderCallback = e), this)
    }
    onAfterRender(e) {
      return (e && (this._onAfterRenderCallback = e), this)
    }
    remove() {
      ;((this._canDraw = !1),
        this.target && this.renderer.bindFrameBuffer(null),
        this._dispose(),
        this.type === "Plane"
          ? this.renderer.removePlane(this)
          : this.type === "ShaderPass" &&
            (this.target && ((this.target._shaderPass = null), this.target.remove(), (this.target = null)),
            this.renderer.removeShaderPass(this)))
    }
    _dispose() {
      if (this.gl) {
        ;(this._geometry && this._geometry.dispose(),
          this.target &&
            this.type === "ShaderPass" &&
            (this.renderer.removeRenderTarget(this.target), this.textures.shift()))
        for (let e = 0; e < this.textures.length; e++) this.textures[e]._dispose()
        this.textures = []
      }
    }
  }
  const We = new A(),
    mt = new A()
  class _t extends gt {
    constructor(
      e,
      t,
      s = "DOMMesh",
      {
        widthSegments: i,
        heightSegments: r,
        renderOrder: a,
        depthTest: h,
        cullFace: o,
        uniforms: l,
        vertexShaderID: d,
        fragmentShaderID: u,
        vertexShader: c,
        fragmentShader: f,
        texturesOptions: p,
        crossOrigin: m,
      } = {},
    ) {
      ;((d = d || (t && t.getAttribute("data-vs-id"))),
        (u = u || (t && t.getAttribute("data-fs-id"))),
        super(e, s, {
          widthSegments: i,
          heightSegments: r,
          renderOrder: a,
          depthTest: h,
          cullFace: o,
          uniforms: l,
          vertexShaderID: d,
          fragmentShaderID: u,
          vertexShader: c,
          fragmentShader: f,
          texturesOptions: p,
          crossOrigin: m,
        }),
        this.gl &&
          ((this.htmlElement = t),
          (!this.htmlElement || this.htmlElement.length === 0) &&
            (this.renderer.production ||
              g(this.type + ": The HTML element you specified does not currently exists in the DOM")),
          this._setDocumentSizes()))
    }
    _setDocumentSizes() {
      let e = this.htmlElement.getBoundingClientRect()
      this._boundingRect || (this._boundingRect = {})
      let t = this.customScale || 1
      this._boundingRect.document = {
        width: e.width * this.renderer.pixelRatio * t,
        height: e.height * this.renderer.pixelRatio * t,
        top: e.top * this.renderer.pixelRatio * t,
        left: e.left * this.renderer.pixelRatio * t,
      }
    }
    getBoundingRect() {
      return {
        width: this._boundingRect.document.width,
        height: this._boundingRect.document.height,
        top: this._boundingRect.document.top,
        left: this._boundingRect.document.left,
        right: this._boundingRect.document.left + this._boundingRect.document.width,
        bottom: this._boundingRect.document.top + this._boundingRect.document.height,
      }
    }
    resize() {
      ;(this._setDocumentSizes(),
        this.type === "Plane" &&
          (this.setPerspective(this.camera.fov, this.camera.near, this.camera.far),
          this._setWorldSizes(),
          this._applyWorldPositions()))
      for (let e = 0; e < this.textures.length; e++) this.textures[e].resize()
      this.renderer.nextRender.add(() => this._onAfterResizeCallback && this._onAfterResizeCallback())
    }
    mouseToPlaneCoords(e) {
      const t = this.scale ? this.scale : mt.set(1, 1),
        s = We.set(
          (this._boundingRect.document.width - this._boundingRect.document.width * t.x) / 2,
          (this._boundingRect.document.height - this._boundingRect.document.height * t.y) / 2,
        ),
        i = {
          width: (this._boundingRect.document.width * t.x) / this.renderer.pixelRatio,
          height: (this._boundingRect.document.height * t.y) / this.renderer.pixelRatio,
          top: (this._boundingRect.document.top + s.y) / this.renderer.pixelRatio,
          left: (this._boundingRect.document.left + s.x) / this.renderer.pixelRatio,
        }
      return We.set(((e.x - i.left) / i.width) * 2 - 1, 1 - ((e.y - i.top) / i.height) * 2)
    }
    onAfterResize(e) {
      return (e && (this._onAfterResizeCallback = e), this)
    }
  }
  class xt {
    constructor({ fov: e = 50, near: t = 0.1, far: s = 150, width: i, height: r, pixelRatio: a = 1 } = {}) {
      ;((this.position = new b()),
        (this.projectionMatrix = new Z()),
        (this.worldMatrix = new Z()),
        (this.viewMatrix = new Z()),
        (this._shouldUpdate = !1),
        this.setSize(),
        this.setPerspective(e, t, s, i, r, a))
    }
    setFov(e) {
      ;((e = isNaN(e) ? this.fov : parseFloat(e)),
        (e = Math.max(1, Math.min(e, 179))),
        e !== this.fov && ((this.fov = e), this.setPosition(), (this._shouldUpdate = !0)),
        this.setCSSPerspective())
    }
    setNear(e) {
      ;((e = isNaN(e) ? this.near : parseFloat(e)),
        (e = Math.max(e, 0.01)),
        e !== this.near && ((this.near = e), (this._shouldUpdate = !0)))
    }
    setFar(e) {
      ;((e = isNaN(e) ? this.far : parseFloat(e)),
        (e = Math.max(e, 50)),
        e !== this.far && ((this.far = e), (this._shouldUpdate = !0)))
    }
    setPixelRatio(e) {
      ;(e !== this.pixelRatio && (this._shouldUpdate = !0), (this.pixelRatio = e))
    }
    setSize(e, t) {
      ;((e !== this.width || t !== this.height) && (this._shouldUpdate = !0), (this.width = e), (this.height = t))
    }
    setPerspective(e, t, s, i, r, a) {
      ;(this.setPixelRatio(a),
        this.setSize(i, r),
        this.setFov(e),
        this.setNear(t),
        this.setFar(s),
        this._shouldUpdate && this.updateProjectionMatrix())
    }
    setPosition() {
      ;(this.position.set(0, 0, 1),
        this.worldMatrix.setFromArray([
          1,
          0,
          0,
          0,
          0,
          1,
          0,
          0,
          0,
          0,
          1,
          0,
          this.position.x,
          this.position.y,
          this.position.z,
          1,
        ]),
        (this.viewMatrix = this.viewMatrix.copy(this.worldMatrix).getInverse()))
    }
    setCSSPerspective() {
      this.CSSPerspective =
        Math.pow(
          Math.pow(this.width / (2 * this.pixelRatio), 2) + Math.pow(this.height / (2 * this.pixelRatio), 2),
          0.5,
        ) / Math.tan((this.fov * 0.5 * Math.PI) / 180)
    }
    getScreenRatiosFromFov(e = 0) {
      const t = this.position.z
      e < t ? (e -= t) : (e += t)
      const s = (this.fov * Math.PI) / 180,
        i = 2 * Math.tan(s / 2) * Math.abs(e)
      return { width: (i * this.width) / this.height, height: i }
    }
    updateProjectionMatrix() {
      const e = this.width / this.height,
        t = this.near * Math.tan((Math.PI / 180) * 0.5 * this.fov),
        s = 2 * t,
        i = e * s,
        r = -0.5 * i,
        a = r + i,
        h = t - s,
        o = (2 * this.near) / (a - r),
        l = (2 * this.near) / (t - h),
        d = (a + r) / (a - r),
        u = (t + h) / (t - h),
        c = -(this.far + this.near) / (this.far - this.near),
        f = (-2 * this.far * this.near) / (this.far - this.near)
      this.projectionMatrix.setFromArray([o, 0, 0, 0, 0, l, 0, 0, d, u, c, -1, 0, 0, f, 0])
    }
    forceUpdate() {
      this._shouldUpdate = !0
    }
    cancelUpdate() {
      this._shouldUpdate = !1
    }
  }
  class _e {
    constructor(e = new Float32Array([0, 0, 0, 1]), t = "XYZ") {
      ;((this.type = "Quat"), (this.elements = e), (this.axisOrder = t))
    }
    setFromArray(e) {
      return (
        (this.elements[0] = e[0]),
        (this.elements[1] = e[1]),
        (this.elements[2] = e[2]),
        (this.elements[3] = e[3]),
        this
      )
    }
    setAxisOrder(e) {
      switch (((e = e.toUpperCase()), e)) {
        case "XYZ":
        case "YXZ":
        case "ZXY":
        case "ZYX":
        case "YZX":
        case "XZY":
          this.axisOrder = e
          break
        default:
          this.axisOrder = "XYZ"
      }
      return this
    }
    copy(e) {
      return ((this.elements = e.elements), (this.axisOrder = e.axisOrder), this)
    }
    clone() {
      return new _e().copy(this)
    }
    equals(e) {
      return (
        this.elements[0] === e.elements[0] &&
        this.elements[1] === e.elements[1] &&
        this.elements[2] === e.elements[2] &&
        this.elements[3] === e.elements[3] &&
        this.axisOrder === e.axisOrder
      )
    }
    setFromVec3(e) {
      const t = e.x * 0.5,
        s = e.y * 0.5,
        i = e.z * 0.5,
        r = Math.cos(t),
        a = Math.cos(s),
        h = Math.cos(i),
        o = Math.sin(t),
        l = Math.sin(s),
        d = Math.sin(i)
      return (
        this.axisOrder === "XYZ"
          ? ((this.elements[0] = o * a * h + r * l * d),
            (this.elements[1] = r * l * h - o * a * d),
            (this.elements[2] = r * a * d + o * l * h),
            (this.elements[3] = r * a * h - o * l * d))
          : this.axisOrder === "YXZ"
            ? ((this.elements[0] = o * a * h + r * l * d),
              (this.elements[1] = r * l * h - o * a * d),
              (this.elements[2] = r * a * d - o * l * h),
              (this.elements[3] = r * a * h + o * l * d))
            : this.axisOrder === "ZXY"
              ? ((this.elements[0] = o * a * h - r * l * d),
                (this.elements[1] = r * l * h + o * a * d),
                (this.elements[2] = r * a * d + o * l * h),
                (this.elements[3] = r * a * h - o * l * d))
              : this.axisOrder === "ZYX"
                ? ((this.elements[0] = o * a * h - r * l * d),
                  (this.elements[1] = r * l * h + o * a * d),
                  (this.elements[2] = r * a * d - o * l * h),
                  (this.elements[3] = r * a * h + o * l * d))
                : this.axisOrder === "YZX"
                  ? ((this.elements[0] = o * a * h + r * l * d),
                    (this.elements[1] = r * l * h + o * a * d),
                    (this.elements[2] = r * a * d - o * l * h),
                    (this.elements[3] = r * a * h - o * l * d))
                  : this.axisOrder === "XZY" &&
                    ((this.elements[0] = o * a * h - r * l * d),
                    (this.elements[1] = r * l * h - o * a * d),
                    (this.elements[2] = r * a * d + o * l * h),
                    (this.elements[3] = r * a * h + o * l * d)),
        this
      )
    }
  }
  const yt = new A(),
    bt = new b(),
    Pt = new b(),
    wt = new b(),
    vt = new b(),
    Tt = new b(),
    Rt = new b(),
    N = new b(),
    H = new b(),
    Ne = new _e(),
    St = new b(0.5, 0.5, 0),
    Et = new b(),
    Mt = new b(),
    Ct = new b(),
    At = new b(),
    kt = new A()
  class He extends _t {
    constructor(
      e,
      t,
      {
        widthSegments: s,
        heightSegments: i,
        renderOrder: r,
        depthTest: a,
        cullFace: h,
        uniforms: o,
        vertexShaderID: l,
        fragmentShaderID: d,
        vertexShader: u,
        fragmentShader: c,
        texturesOptions: f,
        crossOrigin: p,
        alwaysDraw: m = !1,
        visible: T = !0,
        transparent: S = !1,
        drawCheckMargins: R = { top: 0, right: 0, bottom: 0, left: 0 },
        autoloadSources: E = !0,
        watchScroll: M = !0,
        fov: k = 50,
        customScale: C = 1,
      } = {},
    ) {
      ;(super(e, t, "Plane", {
        widthSegments: s,
        heightSegments: i,
        renderOrder: r,
        depthTest: a,
        cullFace: h,
        uniforms: o,
        vertexShaderID: l,
        fragmentShaderID: d,
        vertexShader: u,
        fragmentShader: c,
        texturesOptions: f,
        crossOrigin: p,
      }),
        this.gl &&
          ((this.index = this.renderer.planes.length),
          (this.target = null),
          (this.alwaysDraw = m),
          (this._shouldDraw = !0),
          (this.visible = T),
          (this._transparent = S),
          (this.drawCheckMargins = R),
          (this.autoloadSources = E),
          (this.watchScroll = M),
          (this._updateMVMatrix = !1),
          (this.camera = new xt({
            fov: k,
            width: this.renderer._boundingRect.width,
            height: this.renderer._boundingRect.height,
            pixelRatio: this.renderer.pixelRatio,
          })),
          (this.customScale = C),
          this._program.compiled &&
            (this._initPlane(), this.renderer.scene.addPlane(this), this.renderer.planes.push(this))))
    }
    _programRestored() {
      ;(this.target && this.setRenderTarget(this.renderer.renderTargets[this.target.index]),
        this._initMatrices(),
        this.setPerspective(this.camera.fov, this.camera.near, this.camera.far),
        this._setWorldSizes(),
        this._applyWorldPositions(),
        this.renderer.scene.addPlane(this))
      for (let e = 0; e < this.textures.length; e++)
        ((this.textures[e]._parent = this), this.textures[e]._restoreContext())
      this._canDraw = !0
    }
    _initPlane() {
      ;(this._initTransformValues(),
        this._initPositions(),
        this.setPerspective(this.camera.fov, this.camera.near, this.camera.far),
        this._initSources())
    }
    _initTransformValues() {
      ;((this.rotation = new b()),
        this.rotation.onChange(() => this._applyRotation()),
        (this.quaternion = new _e()),
        (this.relativeTranslation = new b()),
        this.relativeTranslation.onChange(() => this._setTranslation()),
        (this._translation = new b()),
        (this.scale = new b(1)),
        this.scale.onChange(() => {
          ;((this.scale.z = 1), this._applyScale())
        }),
        (this.transformOrigin = new b(0.5, 0.5, 0)),
        this.transformOrigin.onChange(() => {
          ;(this._setWorldTransformOrigin(), (this._updateMVMatrix = !0))
        }))
    }
    resetPlane(e) {
      ;(this._initTransformValues(),
        this._setWorldTransformOrigin(),
        e !== null && e
          ? ((this.htmlElement = e), this.resize())
          : !e &&
            !this.renderer.production &&
            g(
              this.type +
                ": You are trying to reset a plane with a HTML element that does not exist. The old HTML element will be kept instead.",
            ))
    }
    removeRenderTarget() {
      this.target && (this.renderer.scene.removePlane(this), (this.target = null), this.renderer.scene.addPlane(this))
    }
    _initPositions() {
      ;(this._initMatrices(), this._setWorldSizes(), this._applyWorldPositions())
    }
    _initMatrices() {
      const e = new Z()
      this._matrices = {
        world: { matrix: e },
        modelView: {
          name: "uMVMatrix",
          matrix: e,
          location: this.gl.getUniformLocation(this._program.program, "uMVMatrix"),
        },
        projection: {
          name: "uPMatrix",
          matrix: e,
          location: this.gl.getUniformLocation(this._program.program, "uPMatrix"),
        },
        modelViewProjection: { matrix: e },
      }
    }
    _setPerspectiveMatrix() {
      ;(this.camera._shouldUpdate &&
        (this.renderer.useProgram(this._program),
        this.gl.uniformMatrix4fv(this._matrices.projection.location, !1, this._matrices.projection.matrix.elements)),
        this.camera.cancelUpdate())
    }
    setPerspective(e, t, s) {
      ;(this.camera.setPerspective(
        e,
        t,
        s,
        this.renderer._boundingRect.width,
        this.renderer._boundingRect.height,
        this.renderer.pixelRatio,
      ),
        this.renderer.state.isContextLost && this.camera.forceUpdate(),
        (this._matrices.projection.matrix = this.camera.projectionMatrix),
        this.camera._shouldUpdate &&
          (this._setWorldSizes(),
          this._applyWorldPositions(),
          (this._translation.z = this.relativeTranslation.z / this.camera.CSSPerspective)),
        (this._updateMVMatrix = this.camera._shouldUpdate))
    }
    _setMVMatrix() {
      ;(this._updateMVMatrix &&
        ((this._matrices.world.matrix = this._matrices.world.matrix.composeFromOrigin(
          this._translation,
          this.quaternion,
          this.scale,
          this._boundingRect.world.transformOrigin,
        )),
        this._matrices.world.matrix.scale({
          x: this._boundingRect.world.width,
          y: this._boundingRect.world.height,
          z: 1,
        }),
        this._matrices.modelView.matrix.copy(this._matrices.world.matrix),
        (this._matrices.modelView.matrix.elements[14] -= this.camera.position.z),
        (this._matrices.modelViewProjection.matrix = this._matrices.projection.matrix.multiply(
          this._matrices.modelView.matrix,
        )),
        this.alwaysDraw || this._shouldDrawCheck(),
        this.renderer.useProgram(this._program),
        this.gl.uniformMatrix4fv(this._matrices.modelView.location, !1, this._matrices.modelView.matrix.elements)),
        (this._updateMVMatrix = !1))
    }
    _setWorldTransformOrigin() {
      this._boundingRect.world.transformOrigin = new b(
        (this.transformOrigin.x * 2 - 1) * this._boundingRect.world.width,
        -(this.transformOrigin.y * 2 - 1) * this._boundingRect.world.height,
        this.transformOrigin.z,
      )
    }
    _documentToWorldSpace(e) {
      return Pt.set(
        ((e.x * this.renderer.pixelRatio) / this.renderer._boundingRect.width) * this._boundingRect.world.ratios.width,
        -((e.y * this.renderer.pixelRatio) / this.renderer._boundingRect.height) *
          this._boundingRect.world.ratios.height,
        e.z,
      )
    }
    _setWorldSizes() {
      const e = this.camera.getScreenRatiosFromFov()
      ;((this._boundingRect.world = {
        width: ((this._boundingRect.document.width / this.renderer._boundingRect.width) * e.width) / 2,
        height: ((this._boundingRect.document.height / this.renderer._boundingRect.height) * e.height) / 2,
        ratios: e,
      }),
        this._setWorldTransformOrigin())
    }
    _setWorldPosition() {
      const e = {
          x: this._boundingRect.document.width / 2 + this._boundingRect.document.left,
          y: this._boundingRect.document.height / 2 + this._boundingRect.document.top,
        },
        t = {
          x: this.renderer._boundingRect.width / 2 + this.renderer._boundingRect.left,
          y: this.renderer._boundingRect.height / 2 + this.renderer._boundingRect.top,
        }
      ;((this._boundingRect.world.top =
        ((t.y - e.y) / this.renderer._boundingRect.height) * this._boundingRect.world.ratios.height),
        (this._boundingRect.world.left =
          ((e.x - t.x) / this.renderer._boundingRect.width) * this._boundingRect.world.ratios.width))
    }
    setScale(e) {
      if (!e.type || e.type !== "Vec2") {
        this.renderer.production ||
          g(this.type + ": Cannot set scale because the parameter passed is not of Vec2 type:", e)
        return
      }
      ;(e.sanitizeNaNValuesWith(this.scale).max(yt.set(0.001, 0.001)),
        (e.x !== this.scale.x || e.y !== this.scale.y) && (this.scale.set(e.x, e.y, 1), this._applyScale()))
    }
    _applyScale() {
      for (let e = 0; e < this.textures.length; e++) this.textures[e].resize()
      this._updateMVMatrix = !0
    }
    setRotation(e) {
      if (!e.type || e.type !== "Vec3") {
        this.renderer.production ||
          g(this.type + ": Cannot set rotation because the parameter passed is not of Vec3 type:", e)
        return
      }
      ;(e.sanitizeNaNValuesWith(this.rotation),
        e.equals(this.rotation) || (this.rotation.copy(e), this._applyRotation()))
    }
    _applyRotation() {
      ;(this.quaternion.setFromVec3(this.rotation), (this._updateMVMatrix = !0))
    }
    setTransformOrigin(e) {
      if (!e.type || e.type !== "Vec3") {
        this.renderer.production ||
          g(this.type + ": Cannot set transform origin because the parameter passed is not of Vec3 type:", e)
        return
      }
      ;(e.sanitizeNaNValuesWith(this.transformOrigin),
        e.equals(this.transformOrigin) ||
          (this.transformOrigin.copy(e), this._setWorldTransformOrigin(), (this._updateMVMatrix = !0)))
    }
    _setTranslation() {
      let e = bt.set(0, 0, 0)
      ;(this.relativeTranslation.equals(e) || (e = this._documentToWorldSpace(this.relativeTranslation)),
        this._translation.set(
          this._boundingRect.world.left + e.x,
          this._boundingRect.world.top + e.y,
          this.relativeTranslation.z / this.camera.CSSPerspective,
        ),
        (this._updateMVMatrix = !0))
    }
    setRelativeTranslation(e) {
      if (!e.type || e.type !== "Vec3") {
        this.renderer.production ||
          g(this.type + ": Cannot set translation because the parameter passed is not of Vec3 type:", e)
        return
      }
      ;(e.sanitizeNaNValuesWith(this.relativeTranslation),
        e.equals(this.relativeTranslation) || (this.relativeTranslation.copy(e), this._setTranslation()))
    }
    _applyWorldPositions() {
      ;(this._setWorldPosition(), this._setTranslation())
    }
    updatePosition() {
      ;(this._setDocumentSizes(), this._applyWorldPositions())
    }
    updateScrollPosition(e, t) {
      ;(e || t) &&
        ((this._boundingRect.document.top += t * this.renderer.pixelRatio),
        (this._boundingRect.document.left += e * this.renderer.pixelRatio),
        this._applyWorldPositions())
    }
    _getIntersection(e, t) {
      let s = t.clone().sub(e),
        i = e.clone()
      for (; i.z > -1; ) i.add(s)
      return i
    }
    _getNearPlaneIntersections(e, t, s) {
      const i = this._matrices.modelViewProjection.matrix
      if (s.length === 1)
        s[0] === 0
          ? ((t[0] = this._getIntersection(t[1], N.set(0.95, 1, 0).applyMat4(i))),
            t.push(this._getIntersection(t[3], H.set(-1, -0.95, 0).applyMat4(i))))
          : s[0] === 1
            ? ((t[1] = this._getIntersection(t[0], N.set(-0.95, 1, 0).applyMat4(i))),
              t.push(this._getIntersection(t[2], H.set(1, -0.95, 0).applyMat4(i))))
            : s[0] === 2
              ? ((t[2] = this._getIntersection(t[3], N.set(-0.95, -1, 0).applyMat4(i))),
                t.push(this._getIntersection(t[1], H.set(1, 0.95, 0).applyMat4(i))))
              : s[0] === 3 &&
                ((t[3] = this._getIntersection(t[2], N.set(0.95, -1, 0).applyMat4(i))),
                t.push(this._getIntersection(t[0], H.set(-1, 0.95, 0).applyMat4(i))))
      else if (s.length === 2)
        s[0] === 0 && s[1] === 1
          ? ((t[0] = this._getIntersection(t[3], N.set(-1, -0.95, 0).applyMat4(i))),
            (t[1] = this._getIntersection(t[2], H.set(1, -0.95, 0).applyMat4(i))))
          : s[0] === 1 && s[1] === 2
            ? ((t[1] = this._getIntersection(t[0], N.set(-0.95, 1, 0).applyMat4(i))),
              (t[2] = this._getIntersection(t[3], H.set(-0.95, -1, 0).applyMat4(i))))
            : s[0] === 2 && s[1] === 3
              ? ((t[2] = this._getIntersection(t[1], N.set(1, 0.95, 0).applyMat4(i))),
                (t[3] = this._getIntersection(t[0], H.set(-1, 0.95, 0).applyMat4(i))))
              : s[0] === 0 &&
                s[1] === 3 &&
                ((t[0] = this._getIntersection(t[1], N.set(0.95, 1, 0).applyMat4(i))),
                (t[3] = this._getIntersection(t[2], H.set(0.95, -1, 0).applyMat4(i))))
      else if (s.length === 3) {
        let r = 0
        for (let a = 0; a < e.length; a++) s.includes(a) || (r = a)
        ;((t = [t[r]]),
          r === 0
            ? (t.push(this._getIntersection(t[0], N.set(-0.95, 1, 0).applyMat4(i))),
              t.push(this._getIntersection(t[0], H.set(-1, 0.95, 0).applyMat4(i))))
            : r === 1
              ? (t.push(this._getIntersection(t[0], N.set(0.95, 1, 0).applyMat4(i))),
                t.push(this._getIntersection(t[0], H.set(1, 0.95, 0).applyMat4(i))))
              : r === 2
                ? (t.push(this._getIntersection(t[0], N.set(0.95, -1, 0).applyMat4(i))),
                  t.push(this._getIntersection(t[0], H.set(1, -0.95, 0).applyMat4(i))))
                : r === 3 &&
                  (t.push(this._getIntersection(t[0], N.set(-0.95, -1, 0).applyMat4(i))),
                  t.push(this._getIntersection(t[0], H.set(-1 - 0.95, 0).applyMat4(i)))))
      } else for (let r = 0; r < e.length; r++) ((t[r][0] = 1e4), (t[r][1] = 1e4))
      return t
    }
    _getWorldCoords() {
      const e = [wt.set(-1, 1, 0), vt.set(1, 1, 0), Tt.set(1, -1, 0), Rt.set(-1, -1, 0)]
      let t = [],
        s = []
      for (let o = 0; o < e.length; o++) {
        const l = e[o].applyMat4(this._matrices.modelViewProjection.matrix)
        ;(t.push(l), Math.abs(l.z) > 1 && s.push(o))
      }
      s.length && (t = this._getNearPlaneIntersections(e, t, s))
      let i = 1 / 0,
        r = -1 / 0,
        a = 1 / 0,
        h = -1 / 0
      for (let o = 0; o < t.length; o++) {
        const l = t[o]
        ;(l.x < i && (i = l.x), l.x > r && (r = l.x), l.y < a && (a = l.y), l.y > h && (h = l.y))
      }
      return { top: h, right: r, bottom: a, left: i }
    }
    _computeWebGLBoundingRect() {
      const e = this._getWorldCoords()
      let t = {
        top: 1 - (e.top + 1) / 2,
        right: (e.right + 1) / 2,
        bottom: 1 - (e.bottom + 1) / 2,
        left: (e.left + 1) / 2,
      }
      ;((t.width = t.right - t.left),
        (t.height = t.bottom - t.top),
        (this._boundingRect.worldToDocument = {
          width: t.width * this.renderer._boundingRect.width,
          height: t.height * this.renderer._boundingRect.height,
          top: t.top * this.renderer._boundingRect.height + this.renderer._boundingRect.top,
          left: t.left * this.renderer._boundingRect.width + this.renderer._boundingRect.left,
          right:
            t.left * this.renderer._boundingRect.width +
            this.renderer._boundingRect.left +
            t.width * this.renderer._boundingRect.width,
          bottom:
            t.top * this.renderer._boundingRect.height +
            this.renderer._boundingRect.top +
            t.height * this.renderer._boundingRect.height,
        }))
    }
    getWebGLBoundingRect() {
      if (this._matrices.modelViewProjection)
        (!this._boundingRect.worldToDocument || this.alwaysDraw) && this._computeWebGLBoundingRect()
      else return this._boundingRect.document
      return this._boundingRect.worldToDocument
    }
    _getWebGLDrawRect() {
      return (
        this._computeWebGLBoundingRect(),
        {
          top: this._boundingRect.worldToDocument.top - this.drawCheckMargins.top,
          right: this._boundingRect.worldToDocument.right + this.drawCheckMargins.right,
          bottom: this._boundingRect.worldToDocument.bottom + this.drawCheckMargins.bottom,
          left: this._boundingRect.worldToDocument.left - this.drawCheckMargins.left,
        }
      )
    }
    _shouldDrawCheck() {
      const e = this._getWebGLDrawRect()
      Math.round(e.right) <= this.renderer._boundingRect.left ||
      Math.round(e.left) >= this.renderer._boundingRect.left + this.renderer._boundingRect.width ||
      Math.round(e.bottom) <= this.renderer._boundingRect.top ||
      Math.round(e.top) >= this.renderer._boundingRect.top + this.renderer._boundingRect.height
        ? this._shouldDraw &&
          ((this._shouldDraw = !1),
          this.renderer.nextRender.add(() => this._onLeaveViewCallback && this._onLeaveViewCallback()))
        : (this._shouldDraw ||
            this.renderer.nextRender.add(() => this._onReEnterViewCallback && this._onReEnterViewCallback()),
          (this._shouldDraw = !0))
    }
    isDrawn() {
      return this._canDraw && this.visible && (this._shouldDraw || this.alwaysDraw)
    }
    enableDepthTest(e) {
      this._depthTest = e
    }
    _initSources() {
      let e = 0
      if (this.autoloadSources) {
        const t = this.htmlElement.getElementsByTagName("img"),
          s = this.htmlElement.getElementsByTagName("video"),
          i = this.htmlElement.getElementsByTagName("canvas")
        ;(t.length && this.loadImages(t),
          s.length && this.loadVideos(s),
          i.length && this.loadCanvases(i),
          (e = t.length + s.length + i.length))
      }
      ;(this.loader._setLoaderSize(e), (this._canDraw = !0))
    }
    _startDrawing() {
      this._canDraw &&
        (this._onRenderCallback && this._onRenderCallback(),
        this.target
          ? this.renderer.bindFrameBuffer(this.target)
          : this.renderer.state.scenePassIndex === null && this.renderer.bindFrameBuffer(null),
        this._setPerspectiveMatrix(),
        this._setMVMatrix(),
        (this.alwaysDraw || this._shouldDraw) && this.visible && this._draw())
    }
    mouseToPlaneCoords(e) {
      if ((Ne.setAxisOrder(this.quaternion.axisOrder), Ne.equals(this.quaternion) && St.equals(this.transformOrigin)))
        return super.mouseToPlaneCoords(e)
      {
        const t = {
            x: 2 * (e.x / (this.renderer._boundingRect.width / this.renderer.pixelRatio)) - 1,
            y: 2 * (1 - e.y / (this.renderer._boundingRect.height / this.renderer.pixelRatio)) - 1,
          },
          s = this.camera.position.clone(),
          i = Et.set(t.x, t.y, -0.5)
        ;(i.unproject(this.camera), i.sub(s).normalize())
        const r = Mt.set(0, 0, -1)
        r.applyQuat(this.quaternion).normalize()
        const a = At.set(0, 0, 0),
          h = r.dot(i)
        if (Math.abs(h) >= 1e-4) {
          const o = this._matrices.world.matrix.getInverse().multiply(this.camera.viewMatrix),
            l = this._boundingRect.world.transformOrigin.clone().add(this._translation),
            d = Ct.set(this._translation.x - l.x, this._translation.y - l.y, this._translation.z - l.z)
          ;(d.applyQuat(this.quaternion), l.add(d))
          const u = r.dot(l.clone().sub(s)) / h
          ;(a.copy(s.add(i.multiplyScalar(u))), a.applyMat4(o))
        } else a.set(1 / 0, 1 / 0, 1 / 0)
        return kt.set(a.x, a.y)
      }
    }
    onReEnterView(e) {
      return (e && (this._onReEnterViewCallback = e), this)
    }
    onLeaveView(e) {
      return (e && (this._onLeaveViewCallback = e), this)
    }
  }
  class Me {
    constructor(
      e,
      {
        shaderPass: t,
        depth: s = !1,
        clear: i = !0,
        maxWidth: r,
        maxHeight: a,
        minWidth: h = 1024,
        minHeight: o = 1024,
        texturesOptions: l = {},
      } = {},
    ) {
      if (((this.type = "RenderTarget"), (e = (e && e.renderer) || e), !e || e.type !== "Renderer"))
        F(this.type + ": Renderer not passed as first argument", e)
      else if (!e.gl) {
        e.production ||
          F(this.type + ": Unable to create a " + this.type + " because the Renderer WebGL context is not defined")
        return
      }
      ;((this.renderer = e),
        (this.gl = this.renderer.gl),
        (this.index = this.renderer.renderTargets.length),
        (this._shaderPass = t),
        (this._depth = s),
        (this._shouldClear = i),
        (this._maxSize = {
          width: r ? Math.min(this.renderer.state.maxTextureSize / 4, r) : this.renderer.state.maxTextureSize / 4,
          height: a ? Math.min(this.renderer.state.maxTextureSize / 4, a) : this.renderer.state.maxTextureSize / 4,
        }),
        (this._minSize = { width: h * this.renderer.pixelRatio, height: o * this.renderer.pixelRatio }),
        (l = Object.assign(
          {
            sampler: "uRenderTexture",
            isFBOTexture: !0,
            premultiplyAlpha: !1,
            anisotropy: 1,
            generateMipmap: !1,
            floatingPoint: "none",
            wrapS: this.gl.CLAMP_TO_EDGE,
            wrapT: this.gl.CLAMP_TO_EDGE,
            minFilter: this.gl.LINEAR,
            magFilter: this.gl.LINEAR,
          },
          l,
        )),
        (this._texturesOptions = l),
        (this.userData = {}),
        (this.uuid = Se()),
        this.renderer.renderTargets.push(this),
        this.renderer.onSceneChange(),
        this._initRenderTarget())
    }
    _initRenderTarget() {
      ;(this._setSize(), (this.textures = []), this._createFrameBuffer())
    }
    _restoreContext() {
      ;(this._setSize(), this._createFrameBuffer())
    }
    _setSize() {
      this._shaderPass && this._shaderPass._isScenePass
        ? (this._size = { width: this.renderer._boundingRect.width, height: this.renderer._boundingRect.height })
        : (this._size = {
            width: Math.min(this._maxSize.width, Math.max(this._minSize.width, this.renderer._boundingRect.width)),
            height: Math.min(this._maxSize.height, Math.max(this._minSize.height, this.renderer._boundingRect.height)),
          })
    }
    resize() {
      this._shaderPass &&
        (this._setSize(),
        this.textures[0].resize(),
        this.renderer.bindFrameBuffer(this, !0),
        this._depth && this._bindDepthBuffer(),
        this.renderer.bindFrameBuffer(null))
    }
    _bindDepthBuffer() {
      this._depthBuffer &&
        (this.gl.bindRenderbuffer(this.gl.RENDERBUFFER, this._depthBuffer),
        this.gl.renderbufferStorage(
          this.gl.RENDERBUFFER,
          this.gl.DEPTH_COMPONENT16,
          this._size.width,
          this._size.height,
        ),
        this.gl.framebufferRenderbuffer(
          this.gl.FRAMEBUFFER,
          this.gl.DEPTH_ATTACHMENT,
          this.gl.RENDERBUFFER,
          this._depthBuffer,
        ))
    }
    _createFrameBuffer() {
      ;((this._frameBuffer = this.gl.createFramebuffer()),
        this.renderer.bindFrameBuffer(this, !0),
        this.textures.length
          ? ((this.textures[0]._parent = this), this.textures[0]._restoreContext())
          : new ae(this.renderer, this._texturesOptions).addParent(this),
        this.gl.framebufferTexture2D(
          this.gl.FRAMEBUFFER,
          this.gl.COLOR_ATTACHMENT0,
          this.gl.TEXTURE_2D,
          this.textures[0]._sampler.texture,
          0,
        ),
        this._depth && ((this._depthBuffer = this.gl.createRenderbuffer()), this._bindDepthBuffer()),
        this.renderer.bindFrameBuffer(null))
    }
    getTexture() {
      return this.textures[0]
    }
    remove() {
      if (this._shaderPass) {
        this.renderer.production ||
          g(
            this.type +
              ": You're trying to remove a RenderTarget attached to a ShaderPass. You should remove that ShaderPass instead:",
            this._shaderPass,
          )
        return
      }
      ;(this._dispose(), this.renderer.removeRenderTarget(this))
    }
    _dispose() {
      ;(this._frameBuffer && (this.gl.deleteFramebuffer(this._frameBuffer), (this._frameBuffer = null)),
        this._depthBuffer && (this.gl.deleteRenderbuffer(this._depthBuffer), (this._depthBuffer = null)),
        this.textures[0]._dispose(),
        (this.textures = []))
    }
  }
  class Ft extends He {
    constructor(
      e,
      t,
      {
        sampler: s = "uPingPongTexture",
        widthSegments: i,
        heightSegments: r,
        renderOrder: a,
        depthTest: h,
        cullFace: o,
        uniforms: l,
        vertexShaderID: d,
        fragmentShaderID: u,
        vertexShader: c,
        fragmentShader: f,
        texturesOptions: p,
        crossOrigin: m,
        alwaysDraw: T,
        visible: S,
        transparent: R,
        drawCheckMargins: E,
        autoloadSources: M,
        watchScroll: k,
        fov: C,
      } = {},
    ) {
      if (
        ((h = !1),
        (M = !1),
        super(e, t, {
          widthSegments: i,
          heightSegments: r,
          renderOrder: a,
          depthTest: h,
          cullFace: o,
          uniforms: l,
          vertexShaderID: d,
          fragmentShaderID: u,
          vertexShader: c,
          fragmentShader: f,
          texturesOptions: p,
          crossOrigin: m,
          alwaysDraw: T,
          visible: S,
          transparent: R,
          drawCheckMargins: E,
          autoloadSources: M,
          watchScroll: k,
          fov: C,
        }),
        !this.gl)
      )
        return
      ;(this.renderer.scene.removePlane(this),
        (this.type = "PingPongPlane"),
        this.renderer.scene.addPlane(this),
        (this.readPass = new Me(e, { depth: !1, clear: !1, texturesOptions: p })),
        (this.writePass = new Me(e, { depth: !1, clear: !1, texturesOptions: p })),
        this.createTexture({ sampler: s }))
      let y = 0
      ;(this.readPass.getTexture().onSourceUploaded(() => {
        ;(y++, this._checkIfReady(y))
      }),
        this.writePass.getTexture().onSourceUploaded(() => {
          ;(y++, this._checkIfReady(y))
        }),
        this.setRenderTarget(this.readPass),
        (this._onRenderCallback = () => {
          ;(this.readPass &&
            this.writePass &&
            this.textures[0] &&
            this.textures[0]._uploaded &&
            this.setRenderTarget(this.writePass),
            this._onPingPongRenderCallback && this._onPingPongRenderCallback())
        }),
        (this._onAfterRenderCallback = () => {
          ;(this.readPass && this.writePass && this.textures[0] && this.textures[0]._uploaded && this._swapPasses(),
            this._onPingPongAfterRenderCallback && this._onPingPongAfterRenderCallback())
        }))
    }
    _checkIfReady(e) {
      e === 2 &&
        this.renderer.nextRender.add(() => {
          this.textures[0].copy(this.target.getTexture())
        })
    }
    _swapPasses() {
      const e = this.readPass
      ;((this.readPass = this.writePass), (this.writePass = e), this.textures[0].copy(this.readPass.getTexture()))
    }
    getTexture() {
      return this.textures[0]
    }
    onRender(e) {
      return (e && (this._onPingPongRenderCallback = e), this)
    }
    onAfterRender(e) {
      return (e && (this._onPingPongAfterRenderCallback = e), this)
    }
    remove() {
      ;((this.target = null),
        this.renderer.bindFrameBuffer(null),
        this.writePass && (this.writePass.remove(), (this.writePass = null)),
        this.readPass && (this.readPass.remove(), (this.readPass = null)),
        super.remove())
    }
  }
  let xe,
    de,
    Ce = !1,
    re = window.innerHeight || document.documentElement.clientHeight,
    J = window.innerWidth || document.documentElement.clientWidth,
    G = window.scrollY || window.pageYOffset,
    Ae = 0,
    ne = 0,
    ce = 0,
    ue,
    ye = null,
    he
  typeof document.hidden < "u"
    ? ((xe = "hidden"), (de = "visibilitychange"))
    : typeof document.msHidden < "u"
      ? ((xe = "msHidden"), (de = "msvisibilitychange"))
      : typeof document.webkitHidden < "u" && ((xe = "webkitHidden"), (de = "webkitvisibilitychange"))
  function zt(n, e) {
    let t
    return function (...s) {
      ;(clearTimeout(t),
        (t = setTimeout(() => {
          n.apply(this, s)
        }, e)))
    }
  }
  function fe(n) {
    return window.atob(n)
  }
  function pe() {
    return "id-" + Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15)
  }
  function ke(n, e, t, s, i) {
    if (t.length > 1) {
      let r = s ? +s * 2 * Math.PI : 0,
        a = e.width / 2,
        h = e.height / 2,
        o = e.width / 2,
        l = e.height / 2,
        d
      if (i === "radial") {
        const f = Math.max(o, l)
        d = n.createRadialGradient(a, h, f, a, h, 0)
      }
      const u = Math.cos(r) * o,
        c = Math.sin(r) * l
      if ((i === "linear" && (d = n.createLinearGradient(a - u, h - c, a + u, h + c)), i === "conic")) {
        d = n.createConicGradient(-Math.PI + r, a, h)
        const f = [...t, ...t.slice().reverse()]
        f.forEach((p, m) => {
          d.addColorStop(m * (1 / (f.length - 1)), p)
        })
      } else
        t.forEach((f, p) => {
          d.addColorStop(p * (1 / (t.length - 1)), f)
        })
      return d
    }
    return t[0]
  }
  function Ge(n) {
    const e = {
      linear: (t) => t,
      easeInQuad: (t) => t * t,
      easeOutQuad: (t) => 1 - (1 - t) * (1 - t),
      easeInOutQuad: (t) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2),
      easeInCubic: (t) => t * t * t,
      easeOutCubic: (t) => 1 - Math.pow(1 - t, 3),
      easeInOutCubic: (t) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2),
      easeInQuart: (t) => t ** 4,
      easeOutQuart: (t) => 1 - Math.pow(1 - t, 4),
      easeInOutQuart: (t) => (t < 0.5 ? 8 * t ** 4 : 1 - Math.pow(-2 * t + 2, 4) / 2),
      easeInQuint: (t) => t ** 5,
      easeOutQuint: (t) => 1 - Math.pow(1 - t, 5),
      easeInOutQuint: (t) => (t < 0.5 ? 16 * t ** 5 : 1 - Math.pow(-2 * t + 2, 5) / 2),
      easeOutElastic: (t) => {
        const s = (2 * Math.PI) / 3
        return t === 0 ? 0 : t === 1 ? 1 : Math.pow(2, -10 * t) * Math.sin((t * 10 - 0.75) * s) + 1
      },
      easeInElastic: (t) => {
        const s = (2 * Math.PI) / 3
        return t === 0 ? 0 : t === 1 ? 1 : -Math.pow(2, 10 * t - 10) * Math.sin((t * 10 - 10.75) * s)
      },
      easeInOutElastic: (t) => {
        const s = (2 * Math.PI) / 4.5
        return t === 0
          ? 0
          : t === 1
            ? 1
            : t < 0.5
              ? -(Math.pow(2, 20 * t - 10) * Math.sin((20 * t - 11.125) * s)) / 2
              : (Math.pow(2, -20 * t + 10) * Math.sin((20 * t - 11.125) * s)) / 2 + 1
      },
      easeInSine: (t) => 1 - Math.cos((t * Math.PI) / 2),
      easeOutSine: (t) => Math.sin((t * Math.PI) / 2),
      easeInOutSine: (t) => -(Math.cos(Math.PI * t) - 1) / 2,
      easeInCirc: (t) => 1 - Math.sqrt(1 - t ** 2),
      easeOutCirc: (t) => Math.sqrt(1 - Math.pow(t - 1, 2)),
      easeInOutCirc: (t) =>
        t < 0.5 ? (1 - Math.sqrt(1 - (2 * t) ** 2)) / 2 : (Math.sqrt(1 - Math.pow(-2 * t + 2, 2)) + 1) / 2,
      easeInExpo: (t) => (t === 0 ? 0 : Math.pow(2, 10 * t - 10)),
      easeOutExpo: (t) => (t === 1 ? 1 : 1 - Math.pow(2, -10 * t)),
      easeInOutExpo: (t) =>
        t === 0 ? 0 : t === 1 ? 1 : t < 0.5 ? Math.pow(2, 20 * t - 10) / 2 : (2 - Math.pow(2, -20 * t + 10)) / 2,
    }
    return e[n] || e.linear
  }
  function ee(n) {
    return (n && typeof n == "string" && (n = JSON.parse(n)), Object.values(n))
  }
  function be(n, e, t) {
    for (let s = 0; s < t; s++) n = (n + e) / 2
    return +((n + e) / 2).toFixed(4)
  }
  function Pe() {
    return /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)
  }
  function $(n, e) {
    return n.getAttribute(e)
  }
  function oe(n) {
    if (n.local.isDynamicCache !== void 0) return n.local.isDynamicCache
    const e =
        (n.hasOwnProperty("trackMouse") && n.trackMouse !== 0) ||
        (n.hasOwnProperty("axisTilt") && n.axisTilt !== 0) ||
        (n.hasOwnProperty("trackMouseMove") && n.trackMouseMove !== 0),
      t = n.states && [...n.states.appear, ...n.states.scroll, ...n.states.hover].length > 0,
      s = n.layerType === "effect" && (n.animating || n.usesPingPong),
      i = n.type === "video",
      r = e || t || s || i
    return ((n.local.isDynamicCache = r), r)
  }
  function It(n, e, t) {
    const s = []
    return (
      n.forEach((i) => {
        switch (i.layerType) {
          case "text":
            s.push(new jt(i, e, null, t).unpackage())
            break
          case "image":
            s.push(new Gt(i, e, t).unpackage())
            break
          case "shape":
            s.push(new Nt(i, e, t).unpackage())
            break
          case "effect":
            s.push(new Ht(i, e, t).unpackage())
            break
        }
      }),
      s
    )
  }
  function Lt(n, e) {
    const t = document.createElement("a")
    ;((t.href = "https://unicorn.studio?utm_source=public-url"),
      (t.style =
        "position: absolute; display: flex; bottom: 30px; left: 0; width: 190px; margin: 0 auto; right: 0rem; padding: 10px; border-radius: 6px; background-color: rgba(255, 255, 255, 1); box-shadow: 0 3px 9px 0 rgba(0, 0, 0, .2); z-index: 99999999; box-sizing: border-box;"),
      (t.target = "_blank"))
    const s = document.createElement("img")
    ;((s.src = fe("aHR0cHM6Ly9hc3NldHMudW5pY29ybi5zdHVkaW8vbWVkaWEvbWFkZV9pbl91c19zbWFsbF93ZWIuc3Zn")),
      (s.alt = fe("TWFkZSB3aXRoIHVuaWNvcm4uc3R1ZGlv")),
      (s.style = "width: 170px; height: auto;"),
      t.appendChild(s),
      e.appendChild(t))
  }
  function je(n, e) {
    if (!e) return
    let t = n.widthMode === "fixed" ? n.width : n.width * e.offsetWidth,
      s = n.height,
      i = n.fontSizeMode === "fixed" ? n.fontSize : n.fontSize * e.offsetWidth,
      r = n.fontSizeMode === "fixed" ? n.lineHeight : n.lineHeight * i,
      a = n.fontSizeMode === "fixed" ? n.letterSpacing : n.letterSpacing * i,
      h = n.topMode === "fixed" ? n.top : n.top * e.offsetHeight,
      o = n.leftMode === "fixed" ? n.left : n.left * e.offsetWidth,
      l = le(n.anchorPoint)
    ;((o -= l.x * t), (h -= l.y * s), (h += e.offsetTop), (o += e.offsetLeft))
    const d = document.querySelector(`[data-us-text="${n.local.id}"]`),
      u = d || document.createElement("div")
    ;(u.setAttribute("data-us-text", n.local.id),
      (u.style.cssText = `width:${t}px;top:${h}px;left:${o}px;font-size:${i}px;line-height:${r}px;letter-spacing:${a}px;font-family:${n.fontFamily};font-weight:${n.fontWeight};text-align:${n.textAlign};position:absolute;word-break:break-word;transform:rotateZ(${Math.round(n.rotation * 360)}deg);color:transparent;z-index:2;`),
      (u.innerText = n.textContent),
      d || e.appendChild(u))
  }
  function Dt(n) {
    const e = n.split(",").map((r) => r.trim()),
      t = /^((?!chrome|android).)*safari/i.test(navigator.userAgent),
      s = /iPad|iPhone|iPod/.test(navigator.userAgent)
    if (t || s) {
      const r = e.find((a) => a.endsWith(".mov") || a.endsWith(".mp4"))
      if (r) return r
    }
    const i = e.find((r) => r.endsWith(".webm"))
    return i || e[0]
  }
  function Xe() {
    _.scenes.forEach((n, e) => {
      document.body.contains(n.element) || (n.curtain && n.curtain.dispose(), _.scenes.splice(e, 1))
    })
  }
  function Ot(n) {
    return n && n.renderer && n.renderer.nextRender && typeof n.renderer.nextRender.execute == "function"
  }
  function Fe() {
    cancelAnimationFrame(ue)
    let n = _.scenes.filter((t) => t.getDynamicLayers().length > 0)
    const e = (t) => {
      let s = !1
      ;(n.forEach((i) => {
        i.isInView && i.initialized
          ? ((i.rendering = !0),
            t - (i.lastTime || 0) >= i.frameDuration && !i.paused && (i.renderFrame(), (i.lastTime = t)),
            (s = _.scenes.filter((r) => r.getDynamicLayers().length > 0).length > 0))
          : (i.rendering = !1)
      }),
        (!ce || t - ce > 64) &&
          ((G = window.scrollY || window.pageYOffset),
          (ne = G - Ae),
          (Ae = G),
          (ce = t),
          (ne > 1e3 || (ne < 0 && !Ae && !G)) && Le()),
        s ? (Yt(), (ue = requestAnimationFrame(e))) : cancelAnimationFrame(ue))
    }
    n.length && (ue = requestAnimationFrame(e))
  }
  function we(n, e, t) {
    return (n == null ? void 0 : n.type) === "Vec3" && (e == null ? void 0 : e.type) === "Vec3"
      ? new b(n._x + (e._x - n._x) * t, n._y + (e._y - n._y) * t, n._z + (e._z - n._z) * t)
      : (n == null ? void 0 : n.type) === "Vec2" && (e == null ? void 0 : e.type) === "Vec2"
        ? new A(n._x + (e._x - n._x) * t, n._y + (e._y - n._y) * t)
        : n + (e - n) * t
  }
  function ve(n, e) {
    return (
      n.type === "Vec2" ? (n = new A(n._x, e ? 1 - n._y : n._y)) : n.type === "Vec3" && (n = new b(n._x, n._y, n._z)),
      n
    )
  }
  function Te(n, e, t) {
    ;(n == null ? void 0 : n.type) &&
    ((n == null ? void 0 : n.type) === "Vec2" || (n == null ? void 0 : n.type) === "Vec3")
      ? ((e.value.x = n.x),
        t === "pos" ? (e.value.y = 1 - n.y) : (e.value.y = n.y),
        n.type === "Vec3" && (e.value.z = n.z))
      : (e.value = n)
  }
  function j(n, e, t) {
    n.createTexture({ sampler: e, premultiplyAlpha: !0, fromTexture: t })
  }
  function ze(n, e, t) {
    if (ye) {
      n.loadImage(ye, { sampler: "uTexture" })
      return
    }
    const s = document.createElement("canvas")
    ;((s.width = e * 0.01), (s.height = t * 0.01))
    const i = s.getContext("2d")
    ;((i.fillStyle = "rgba(0, 0, 0, 0)"),
      i.fillRect(0, 0, e * 0.01, t * 0.01),
      (ye = s.toDataURL()),
      n.loadImage(ye, { sampler: "uTexture" }))
  }
  class Ut {
    constructor({ prop: e, value: t, endValue: s, transition: i, breakpoints: r, uniformData: a }) {
      L(this, "id", pe())
      ;((this.prop = e),
        (this.transition = i),
        (this.complete = !1),
        (this.progress = 0),
        (this.uniformData = a),
        (this.value = ve(t)),
        (this.endValue = s !== void 0 ? ve(s || 0) : null),
        (this.lastTick = void 0),
        (this.initialized = !1),
        (this.startTime = null),
        (this.breakpoints = r || []))
    }
    updateEffect(e, t) {
      if (((e = this.endValue ?? e), e === void 0)) return !1
      const s = performance.now(),
        i = Ge(this.transition.ease),
        r = (this.startTime || t) + this.transition.delay,
        a = Math.max(0, Math.min(1, (s - r) / this.transition.duration))
      return (
        (this.progress = i(a)),
        this.progress === 0
          ? !1
          : (a >= 1 && ((this.complete = !0), (this.progress = 0)),
            (this.lastTick = s),
            this.complete ? !1 : we(this.value, e, this.progress))
      )
    }
    resetState() {
      ;((this.progress = 0), (this.complete = !1), (this.lastTick = void 0), (this.initialized = !1))
    }
  }
  class Vt {
    constructor({
      prop: e,
      value: t,
      range: s,
      offset: i,
      momentum: r,
      uniformData: a,
      mode: h = "scrollIntoView",
      delta: o = 0.01,
      absScrollValue: l = !0,
      breakpoints: d = [],
    }) {
      L(this, "id", pe())
      ;((this.prop = e),
        (this.progress = 0),
        (this.momentum = r),
        (this.range = s),
        (this.offset = i),
        (this.mode = h),
        (this.delta = o),
        (this.absScrollValue = l),
        (this.uniformData = a),
        (this.value = ve(t)),
        (this.lastTick = void 0),
        (this.breakpoints = d || []))
    }
    updateEffect(e, { top: t, height: s, isFixed: i }) {
      if (e === void 0) return !1
      if ((i && (t -= G), this.mode === "scrollIntoView")) {
        const r = t + G - re * this.offset,
          a = r + (re + s) * this.range
        let h = (G - r) / (a - r),
          o = Math.max(0, Math.min(1, h))
        return (
          this.lastTick !== void 0 && (o = be(o, this.lastTick, this.momentum * 2)),
          this.lastTick !== void 0 && Math.abs(this.lastTick - o) < 1e-4
            ? !1
            : ((this.lastTick = o), we(e, this.value, o))
        )
      } else if (this.mode === "whileScrolling") {
        let r = ne * this.delta
        return (
          this.absScrollValue && (r = Math.abs(r)),
          this.lastTick !== void 0 && ((r = be(r, this.lastTick, this.momentum * 2)), Math.abs(r) < 0.001)
            ? !1
            : ((this.lastScrollPos = G), (this.lastTick = r), we(e, this.value, r))
        )
      }
      return !1
    }
    resetState() {
      this.lastTick = void 0
    }
  }
  class Bt {
    constructor({ prop: e, value: t, transition: s, breakpoints: i, uniformData: r, triggerOnElement: a }) {
      L(this, "id", pe())
      ;((this.prop = e),
        (this.transition = s),
        (this.progress = 0),
        (this.rawProgress = 0),
        (this.lastProgress = null),
        (this.value = ve(t)),
        (this.uniformData = r),
        (this.triggerOnElement = a !== void 0 ? a : 1),
        (this.breakpoints = i || []))
    }
    updateEffect(e, t, s) {
      var c
      if (e === void 0) return !1
      const i = (c = s == null ? void 0 : s.getParent) == null ? void 0 : c.call(s),
        r = performance.now()
      if (this.triggerOnElement && ((s != null && s.isElement) || i)) {
        let f = (i || s).isHovered()
        f && !this.lastEnterTime ? (t = r) : f ? (t = this.lastEnterTime) : (t = null)
      }
      const a = t === null,
        h = a ? this.lastTick || r : t + this.transition.delay,
        o = (r - h) / this.transition.duration,
        l = Math.max(0, Math.min(1, o))
      a
        ? (this.rawProgress = Math.max(0, this.rawProgress - l))
        : (this.lastEnterTime !== t && (this.lastProgress = this.rawProgress || 0),
          (this.rawProgress = Math.min(1, this.lastProgress + l)))
      const d = Ge(this.transition.ease)(this.rawProgress),
        u = Math.abs(this.progress - d) > 1e-7
      return (
        (this.progress = d),
        !t && !u
          ? !1
          : (!t && this.transition.forwardsOnly && ((this.progress = 0), (this.rawProgress = 0)),
            (this.lastTick = r),
            (this.lastEnterTime = t),
            u ? we(e, this.value, this.progress) : !1)
      )
    }
    resetState() {
      this.progress = 0
    }
  }
  function Wt(n) {
    return (
      n.forEach((e) => {
        var t, s
        for (let i in e.props)
          ((t = e.props[i]) == null ? void 0 : t.type) === "Vec2"
            ? (e.props[i] = new A(e.props[i]._x, e.props[i]._y))
            : ((s = e.props[i]) == null ? void 0 : s.type) === "Vec3"
              ? (e.props[i] = new b(e.props[i]._x, e.props[i]._y, e.props[i]._z))
              : typeof e.props[i] == "object" && (e.props[i] = ee(e.props[i]))
      }),
      n
    )
  }
  class Ye {
    constructor(e, t) {
      L(this, "local", { id: "", projectId: "", lastMousePos: { x: 0.5, y: 0.5 }, createdAt: null })
      ;((this.locked = e.locked || !1),
        (this.aspectRatio = e.aspectRatio || 1),
        (this.layerName = e.layerName || ""),
        (this.data = e.data || {}),
        (this.userDownsample = e.userDownsample || 1),
        (this.breakpoints = Wt(e.breakpoints || [])),
        (this.local.sceneId = t),
        (this.local.id = pe()),
        (this.states = {
          appear: e.states && e.states.appear ? e.states.appear.map((s) => new Ut(s)) : [],
          scroll: e.states && e.states.scroll ? e.states.scroll.map((s) => new Vt(s)) : [],
          hover: e.states && e.states.hover ? e.states.hover.map((s) => new Bt(s)) : [],
        }))
    }
    state() {
      return _.scenes.find((e) => e.id === this.local.sceneId) || this.initOptions
    }
    getIndex() {
      return this.state()
        .layers.map((e) => e.local.id)
        .indexOf(this.local.id)
    }
    getPlane() {
      var e, t, s
      return (s = (t = (e = this.state()) == null ? void 0 : e.curtain) == null ? void 0 : t.planes) == null
        ? void 0
        : s.find((i) => i.type !== "PingPongPlane" && i.userData.id === this.local.id)
    }
    getPlanes() {
      var e, t, s
      return (
        ((s = (t = (e = this.state()) == null ? void 0 : e.curtain) == null ? void 0 : t.planes) == null
          ? void 0
          : s.filter((i) => i.type !== "PingPongPlane" && i.userData.id === this.local.id)) || []
      )
    }
    getMaskedItem() {
      return this.mask ? this.state().layers.filter((e) => !e.parentLayer)[this.getIndex() - 1] : !1
    }
    getChildEffectItems() {
      if (this.effects && this.effects.length) {
        const e = this.state()
        if (!e || !e.layers) return []
        const t = e.layers.filter((i) => this.effects.includes(i.parentLayer))
        return this.effects.map((i) => t.find((r) => r.parentLayer === i)).filter((i) => i !== void 0)
      } else return []
    }
    setBreakpointValues() {
      const e = J,
        t = this.breakpoints.sort((r, a) => a.min - r.min),
        s = {}
      if (t.length === 1 && t[0].name === "Desktop") return
      if (t.length >= 1 && !t.find((r) => r.name === "Desktop"))
        throw new Error("Malfored breakpoint data, missing Desktop")
      for (let r = t.length - 1; r >= 0; r--) {
        const a = t[r]
        if (a.max === null || e <= a.max) for (let h in a.props) s.hasOwnProperty(h) || (s[h] = a.props[h])
      }
      const i = this.breakpoints.find((r) => r.name === "Desktop")
      if (i) for (let r in i.props) s.hasOwnProperty(r) || (s[r] = i.props[r])
      for (let r in s)
        if (this.hasOwnProperty(r)) {
          let a = s[r]
          a != null && a.type
            ? ((this[r].x = a._x), (this[r].y = a._y), a._z !== void 0 && (this[r].z = a._z))
            : (this[r] = a)
        }
      this.local.bpProps = s
    }
    updateStateEffects() {
      var s, i, r, a
      if (
        !this.states.appear.some((h) => !h.complete) &&
        !((s = this.states) != null && s.scroll.length) &&
        !((i = this.states) != null && i.hover.length)
      )
        return
      ;(this.local.uniformUpdatesPool || (this.local.uniformUpdatesPool = {}),
        this.local.updatedProps ? this.local.updatedProps.clear() : (this.local.updatedProps = new Set()),
        (this.local.hasUpdates = !1))
      const e = this.state()
      ;(!this.local.cachedBreakpoint ||
        this.local.cachedBreakpoint !== ((r = e.currentBreakpoint) == null ? void 0 : r.name)) &&
        (this.local.cachedBreakpoint = (a = e.currentBreakpoint) == null ? void 0 : a.name)
      const t = (h) =>
        !h.breakpoints || h.breakpoints.length === 0 ? !0 : h.breakpoints.find((o) => o === this.local.cachedBreakpoint)
      ;(e.isInView &&
        (this.local.createdAt === null && (this.local.createdAt = performance.now()),
        this.states.appear.forEach((h) => {
          if (!t(h)) return
          let o = h.startTime || this.local.createdAt
          if (!h.initialized) {
            const d = [...this.states.appear.filter((u) => u.prop === h.prop)].sort(
              (u, c) => (u.transition.delay || 0) - (c.transition.delay || 0),
            )[0]
            ;((d == null ? void 0 : d.id) === h.id &&
              ((this.local.uniformUpdatesPool[h.prop] = h.value),
              (this.local.hasUpdates = !0),
              this.local.updatedProps.add(h.prop)),
              (h.initialized = !0),
              o || (h.startTime = performance.now()))
          }
          e != null && e.pauseTime && (o += Math.abs(e == null ? void 0 : e.pauseTime))
          const l = h.updateEffect(this[h.prop], o)
          ;(l !== !1 &&
            ((this.local.uniformUpdatesPool[h.prop] = l),
            (this.local.hasUpdates = !0),
            this.local.updatedProps.add(h.prop)),
            this.isElement && h.complete && this.removeStateEffectProperty(h.prop))
        })),
        this.states.hover.forEach((h) => {
          if (!t(h)) return
          const o = h.updateEffect(this[h.prop], e.mouse.enterTime, this)
          o !== !1 &&
            ((this.local.uniformUpdatesPool[h.prop] = o),
            (this.local.hasUpdates = !0),
            this.local.updatedProps.add(h.prop))
        }),
        this.states.scroll.forEach((h) => {
          var d
          if (!t(h)) return
          const o = e.bbox
          if (!o) return
          const l = h.updateEffect(this[h.prop], {
            top: e.isFixed ? 0 : (d = e.bbox) == null ? void 0 : d.top,
            height: o.height / 2,
            isFixed: e.isFixed,
          })
          l !== !1 &&
            ((this.local.uniformUpdatesPool[h.prop] = l),
            (this.local.hasUpdates = !0),
            this.local.updatedProps.add(h.prop))
        }),
        this.local.hasUpdates &&
          (this.getPlanes().forEach((h) => {
            var o
            for (let l of this.local.updatedProps) {
              const d = this.local.uniformUpdatesPool[l]
              d !== void 0 &&
                ((o = h.uniforms[l]) == null ? void 0 : o.lastValue) !== d &&
                (h.uniforms[l] && Te(d, h.uniforms[l], l), this.updateStateEffectProperty(l, d))
            }
          }),
          this.isElement &&
            this.local.stateEffectProps &&
            Object.keys(this.local.stateEffectProps).length &&
            this.render()))
    }
    updateStateEffectProperty(e, t) {
      ;(this.local.stateEffectProps || (this.local.stateEffectProps = {}), (this.local.stateEffectProps[e] = t))
    }
    removeStateEffectProperty(e) {
      this.local.stateEffectProps && this.local.stateEffectProps[e] && delete this.local.stateEffectProps[e]
    }
    invalidatePerformanceCaches() {
      ;((this.local.isDynamicCache = void 0),
        (this.local.cachedBreakpoint = void 0),
        (this.local.lastCalculatedMousePos = void 0),
        (this.local.lastCalculatedMouseValues = void 0))
    }
  }
  function le(n) {
    const e = {
      topLeft: { x: 0, y: 0 },
      topCenter: { x: 0.5, y: 0 },
      topRight: { x: 1, y: 0 },
      rightCenter: { x: 1, y: 0.5 },
      bottomRight: { x: 1, y: 1 },
      bottomCenter: { x: 0.5, y: 1 },
      bottomLeft: { x: 0, y: 1 },
      leftCenter: { x: 0, y: 0.5 },
      center: { x: 0.5, y: 0.5 },
    }
    return e[n] || e.topLeft
  }
  let Ie = class extends Ye {
    constructor(t, s, i) {
      super(t, s)
      L(this, "isElement", !0)
      ;((this.anchorPoint = t.anchorPoint || "topLeft"),
        (this.initOptions = i),
        (this.opacity = t.opacity ?? 1),
        (this.displace = t.displace || 0),
        (this.effects = t.effects || []),
        (this.trackMouse = t.trackMouse || 0),
        (this.axisTilt = t.axisTilt || 0),
        (this.bgDisplace = t.bgDisplace || 0),
        (this.dispersion = t.dispersion || 0),
        (this.rotation = t.rotation || 0),
        (this.mouseMomentum = t.mouseMomentum || 0),
        (this.maskAlpha = t.maskAlpha || 0),
        (this.maskDepth = t.maskDepth || 0),
        (this.maskBackground = t.maskBackground || {}),
        (this.mask = t.mask || 0),
        (this.blendMode = t.blendMode || "NORMAL"),
        (this.compiledFragmentShaders = t.compiledFragmentShaders || []),
        (this.compiledVertexShaders = t.compiledVertexShaders || []),
        (this.width = t.width || 0),
        (this.height = t.height || 0),
        (this.widthMode = t.widthMode || "fixed"),
        (this.heightMode = t.heightMode || "fixed"),
        (this.left = t.left || 0),
        (this.top = t.top || 0),
        (this.leftMode = t.leftMode || "relative"),
        (this.topMode = t.topMode || "relative"))
    }
    unpackage() {
      return (
        (this.effects = ee(this.effects)),
        (this.maskBackground = new b(this.maskBackground._x, this.maskBackground._y, this.maskBackground._z)),
        this.unpackageType(),
        this
      )
    }
    getPixelRatio() {
      const t = this.state()
      return t.dpi * t.scale * this.userDownsample
    }
    createLocalCanvas() {
      const t = this.state(),
        s = document.createElement("canvas"),
        i = this.getPixelRatio()
      ;((s.width = t.canvasWidth * i), (s.height = t.canvasHeight * i))
      const r = s.getContext("2d")
      ;(r.scale(i, i), (this.local.canvas = s), (this.local.ctx = r))
    }
    resize() {
      const t = this.state()
      if (this.local.canvas) {
        const s = this.getPixelRatio()
        ;((this.local.canvas.width = t.canvasWidth * s),
          (this.local.canvas.height = t.canvasHeight * s),
          this.local.ctx.scale(s, s))
      }
      ;(this.render(), this.layerType === "text" && je(this, this.state().element))
    }
    dispose() {
      ;(this.local.canvas &&
        ((this.local.canvas.width = 1), (this.local.canvas.height = 1), (this.local.canvas = null)),
        this.local.ctx && (this.local.ctx = null),
        this.invalidatePerformanceCaches())
    }
    box() {
      const t = le(this.anchorPoint),
        { width: s, height: i } = this.getAbsoluteDimensions(),
        { left: r, top: a } = this.getAbsolutePosition(),
        h = s * (0.5 - t.x),
        o = i * (0.5 - t.y)
      return {
        anchor: { x: r + h, y: a + o },
        center: { x: r + s / 2, y: a + i / 2 },
        left: r,
        top: a,
        width: s,
        height: i,
      }
    }
    getAbsolutePosition() {
      var l, d, u, c
      const t = this.state(),
        s = t.canvasWidth,
        i = t.canvasHeight
      let r = ((d = (l = this.local) == null ? void 0 : l.stateEffectProps) == null ? void 0 : d.left) ?? this.left,
        a = ((c = (u = this.local) == null ? void 0 : u.stateEffectProps) == null ? void 0 : c.top) ?? this.top
      const h = this.leftMode === "fixed" ? Math.round(r) : Math.round(r * s),
        o = this.topMode === "fixed" ? Math.round(a) : Math.round(a * i)
      return { left: h, top: o }
    }
    getAbsoluteDimensions() {
      var l, d, u, c
      const t = this.state()
      let s, i
      const r = t.canvasWidth,
        a = t.canvasHeight
      let h = ((d = (l = this.local) == null ? void 0 : l.stateEffectProps) == null ? void 0 : d.width) ?? this.width,
        o = ((c = (u = this.local) == null ? void 0 : u.stateEffectProps) == null ? void 0 : c.height) ?? this.height
      return (
        this.widthMode !== "auto" &&
          ((s = this.widthMode === "fixed" ? h : h * r),
          this.heightMode === "auto" &&
            ((i = s / (this.aspectRatio || 1)), (this.height = this.widthMode === "fixed" ? i : i / a))),
        this.heightMode !== "auto" &&
          ((i = this.heightMode === "fixed" ? o : o * a),
          this.widthMode === "auto" &&
            ((s = i * (this.aspectRatio || 1)), (this.width = this.widthMode === "fixed" ? s : s / r))),
        this.layerType === "text" && (i = this.height),
        { width: s, height: i }
      )
    }
    isHovered() {
      let { left: t, top: s, width: i, height: r } = this.box()
      const a = le(this.anchorPoint)
      ;(i < 0 && (i = Math.abs(i)), r < 0 && ((s = s + r), (r = Math.abs(r))), (t = t - a.x * i), (s = s - a.y * r))
      let h = this.state().mouse.movePos
      return h.x * 2 >= t && h.x * 2 <= t + i && h.y * 2 >= s && h.y * 2 <= s + r
    }
  }
  class Nt extends Ie {
    constructor(t, s, i) {
      super(t, s)
      L(this, "layerType", "shape")
      L(this, "isElement", !0)
      this.initOptions = i
      let r = this.default(t || {})
      for (let a in r) this[a] = r[a]
      ;(this.breakpoints.length && this.setBreakpointValues(), Object.keys(t).length && this.createLocalCanvas())
    }
    default(t) {
      return {
        borderRadius: t.borderRadius || 0,
        fill: t.fill || ["#777777"],
        fitToCanvas: t.fitToCanvas || !1,
        gradientAngle: t.gradientAngle || t.gradAngle || 0,
        gradientType: t.gradientType || t.gradType || "linear",
        numSides: t.numSides || 3,
        type: t.type || "rectangle",
        stroke: t.stroke || ["#000000"],
        strokeWidth: t.strokeWidth || 0,
      }
    }
    unpackageType() {
      return ((this.fill = ee(this.fill)), (this.stroke = ee(this.stroke)), this.render(), this)
    }
    render() {
      var f, p
      let t = this.local.ctx
      const s = this.state()
      let { left: i, top: r, width: a, height: h } = this.box()
      const o = le(this.anchorPoint)
      let l = ((f = this.local.stateEffectProps) == null ? void 0 : f.borderRadius) ?? this.borderRadius,
        d = ((p = this.local.stateEffectProps) == null ? void 0 : p.strokeWidth) ?? this.strokeWidth,
        u = i - o.x * a,
        c = r - o.y * h
      if ((t.clearRect(0, 0, s.canvasWidth, s.canvasHeight), t.beginPath(), this.fitToCanvas))
        (t.rect(0, 0, s.canvasWidth, s.canvasHeight),
          (t.fillStyle = ke(
            t,
            { width: s.canvasWidth, height: s.canvasHeight },
            this.fill,
            this.gradientAngle,
            this.gradientType,
          )))
      else {
        if (
          (t.save(),
          t.translate(u + a / 2, c + h / 2),
          t.rotate(this.rotation * 2 * Math.PI),
          t.translate(-a / 2, -h / 2),
          this.type === "rectangle")
        ) {
          let m = (l * Math.min(Math.abs(a), Math.abs(h))) / 2
          this.drawRoundedRect(t, 0, 0, a, h, m)
        } else
          this.type === "circle"
            ? t.ellipse(a / 2, h / 2, Math.abs(a) / 2, Math.abs(h) / 2, 0, 0, 2 * Math.PI)
            : this.type === "polygon" && this.drawPolygon(t, Math.abs(a), Math.abs(h), this.numSides)
        t.fillStyle = ke(t, this.box(), this.fill, this.gradientAngle, this.gradientType)
      }
      ;(t.fill(), d && ((t.strokeStyle = this.stroke[0]), (t.lineWidth = d), t.stroke()), t.restore())
    }
    drawRoundedRect(t, s, i, r, a, h) {
      ;(t.beginPath(),
        t.moveTo(s + h, i),
        t.arcTo(s + r, i, s + r, i + a, h),
        t.arcTo(s + r, i + a, s, i + a, h),
        t.arcTo(s, i + a, s, i, h),
        t.arcTo(s, i, s + r, i, h),
        t.closePath())
    }
    drawPolygon(t, s, i, r) {
      t.beginPath()
      for (let a = 0; a < r; a++) {
        const h = (a * 2 * Math.PI) / r - Math.PI / 2,
          o = s * Math.cos(h),
          l = i * Math.sin(h)
        a === 0 ? t.moveTo(o, l) : t.lineTo(o, l)
      }
      t.closePath()
    }
  }
  class Ht extends Ye {
    constructor(t, s, i) {
      super(t, s)
      L(this, "layerType", "effect")
      ;((this.initOptions = i),
        (this.type = t.type || "sine"),
        (this.usesPingPong = t.usesPingPong || !1),
        (this.speed = t.speed || 0.5),
        (this.parentLayer = t.parentLayer || !1),
        (this.animating = t.animating || !1),
        (this.isMask = t.isMask || 0),
        (this.texture = t.texture || null),
        (this.mouseMomentum = t.mouseMomentum || 0),
        (this.compiledFragmentShaders = t.compiledFragmentShaders || []),
        (this.compiledVertexShaders = t.compiledVertexShaders || []))
      for (let r in t) this[r] || (this[r] = t[r])
      this.breakpoints.length && this.setBreakpointValues()
    }
    unpackage() {
      for (let t in this)
        this[t] &&
          this[t].type &&
          (this[t].type === "Vec2"
            ? (this[t] = new A(this[t]._x, this[t]._y))
            : this[t].type === "Vec3" && (this[t] = new b(this[t]._x, this[t]._y, this[t]._z)))
      return this
    }
    getParent() {
      return this.state()
        .layers.filter((t) => t.effects && t.effects.length)
        .find((t) => t.effects.includes(this.parentLayer))
    }
  }
  class Gt extends Ie {
    constructor(t, s, i) {
      super(t, s)
      L(this, "layerType", "image")
      L(this, "isElement", !0)
      this.initOptions = i
      let r = this.default(t || {})
      for (let a in r) this[a] = r[a]
      ;(this.breakpoints.length && this.setBreakpointValues(),
        Object.keys(t).length && (this.createLocalCanvas(), this.loadImage()))
    }
    default(t) {
      return { fitToCanvas: t.fitToCanvas || !1, src: t.src || "" }
    }
    unpackage() {
      return ((this.effects = ee(this.effects)), this)
    }
    loadImage() {
      const t = new Image()
      ;((t.crossOrigin = "Anonymous"),
        t.addEventListener(
          "load",
          () => {
            const s = this.state()
            ;((this.local.img = t),
              (this.render = this.renderImage),
              this.render(),
              s.renderFrame && s.renderFrame(),
              (this.local.loaded = !0))
          },
          !1,
        ),
        (t.src = this.src))
    }
    renderImage() {
      var d, u
      if (!((d = this.local) != null && d.ctx)) return
      let t = this.local.ctx
      const s = this.state()
      t.clearRect(0, 0, s.canvasWidth, s.canvasHeight)
      let { left: i, top: r, width: a, height: h } = this.box(),
        o = s.canvasWidth,
        l = s.canvasHeight
      if (this.fitToCanvas) {
        let c = this.local.img.width / this.local.img.height
        o / l < c ? ((h = l), (a = l * c)) : ((a = o), (h = o / c))
        const p = (o - a) / 2,
          m = (l - h) / 2
        t.drawImage(this.local.img, p, m, a, h)
      } else {
        const c = le(this.anchorPoint),
          f = (((u = this.local.stateEffectProps) == null ? void 0 : u.rotation) ?? this.rotation) * 2 * Math.PI
        let p = c.x * a,
          m = c.y * h
        t.save()
        let T = a / 2,
          S = h / 2,
          R = i - p + T,
          E = r - m + S
        ;(t.translate(R, E), t.rotate(f), t.drawImage(this.local.img, -T, -S, a, h), t.restore())
      }
    }
    render() {}
  }
  class jt extends Ie {
    constructor(t, s, i, r) {
      super(t, s)
      L(this, "layerType", "text")
      this.initOptions = r
      let a = this.default(t || {})
      for (let h in a) this[h] = a[h]
      ;(this.breakpoints.length && this.setBreakpointValues(),
        (this.isSafari = /^((?!chrome|android).)*safari/i.test(navigator.userAgent)),
        Object.keys(t).length && this.createLocalCanvas(),
        this.loadFont())
    }
    default(t) {
      return {
        fill: t.fill || ["#ffffff"],
        fontSize: t.fontSize ?? 24,
        fontSizeMode: t.fontSizeMode || "fixed",
        fontCSS: t.fontCSS || null,
        lineHeight: t.lineHeight ?? 25,
        letterSpacing: t.letterSpacing || 0,
        fontFamily: t.fontFamily || "arial",
        fontStyle: t.fontStyle || "normal",
        fontWeight: t.fontWeight || "normal",
        textAlign: t.textAlign || "left",
        textContent: t.textContent || "",
        gradientAngle: t.gradientAngle || t.gradAngle || 0,
        gradientType: t.gradientType || t.gradType || "linear",
      }
    }
    unpackage() {
      return ((this.fill = ee(this.fill)), (this.effects = ee(this.effects)), this)
    }
    loadFont() {
      var o
      const t = this.fontStyle.includes("italic") ? "italic" : "normal",
        s = (l) => (l === "normal" || l === "400" || l === 400 ? "normal" : l.toString()),
        i = isNaN(parseInt(this.fontWeight)) ? "normal" : s(this.fontWeight)
      if (
        Array.from(document.fonts).some(
          (l) => l.family === this.fontFamily && l.style === t && s(l.weight) === i && l.status === "loaded",
        ) ||
        !((o = this.fontCSS) != null && o.src)
      ) {
        this.handleFontLoaded()
        return
      }
      let a = this.fontCSS.src.split(" ").join("%20")
      const h = new FontFace(this.fontFamily, `url(${a})`, { style: t, weight: i })
      ;(document.fonts.add(h),
        h
          .load()
          .then(() => {
            this.handleFontLoaded()
          })
          .catch((l) => {
            ;(console.error("Font loading error:", l), this.handleFontLoaded())
          }))
    }
    handleFontLoaded() {
      ;((this.local.loaded = !0), this.render(), this.render())
      const t = this.state()
      ;(t.renderFrame && t.renderFrame(), je(this, this.state().element))
    }
    getAbsoluteFontValues(t) {
      var h, o, l, d, u, c
      const s =
          ((o = (h = this.local) == null ? void 0 : h.stateEffectProps) == null ? void 0 : o.fontSize) || this.fontSize,
        i =
          ((d = (l = this.local) == null ? void 0 : l.stateEffectProps) == null ? void 0 : d.lineHeight) ||
          this.lineHeight,
        r =
          ((c = (u = this.local) == null ? void 0 : u.stateEffectProps) == null ? void 0 : c.letterSpacing) ||
          this.letterSpacing,
        a = this.fontSizeMode === "fixed" ? s : s * t.canvasWidth
      return {
        fontSize: a,
        lineHeight: this.fontSizeMode === "fixed" ? i : i * a,
        letterSpacing: this.fontSizeMode === "fixed" ? r : r * a,
      }
    }
    render() {
      var U, V, q
      if (!((U = this.local) != null && U.ctx)) return
      let t = this.local.ctx,
        { left: s, top: i, width: r } = this.box()
      const a = le(this.anchorPoint),
        h =
          ((q = (V = this.local) == null ? void 0 : V.stateEffectProps) == null ? void 0 : q.rotation) || this.rotation
      let o = this.state(),
        l = this.height,
        d = s - a.x * r,
        u = i - a.y * l,
        c = 0,
        { fontSize: f, lineHeight: p, letterSpacing: m } = this.getAbsoluteFontValues(o),
        T = this.fontStyle.includes("italic") ? "italic" : "normal",
        S = /^\d+$/.test(this.fontStyle) ? this.fontStyle : "400"
      ;(t.clearRect(0, 0, o.canvasWidth, o.canvasHeight),
        (t.font = `${T} ${S} ${f}px/${p}px ${this.fontFamily}, -apple-system, BlinkMacSystemFont, Helvetica, Arial`),
        this.isSafari || ((t.textAlign = this.textAlign), (t.letterSpacing = m + "px")))
      const R = t.measureText("m").width
      ;((r = Math.max(r, R)), t.save())
      let E = d + r / 2,
        M = u + l / 2
      ;(t.translate(E, M),
        t.rotate((h * 360 * Math.PI) / 180),
        t.translate(-E, -M),
        this.textAlign === "center" && (d += r / 2),
        this.textAlign === "right" && (d += r),
        (t.fillStyle = ke(t, this.box(), this.fill, this.gradientAngle, this.gradientType)))
      const k = (P, w, v, x, z, Y, ie) => {
          let B = w.split("").reduce((I, K, ss) => I + P.measureText(K).width + (ss < w.length - 1 ? z : 0), 0),
            W
          if ((Y === "center" ? (W = v + (ie - B) / 2 - ie / 2) : (W = v), Y === "right"))
            for (let I = w.length - 1; I >= 0; I--) {
              const K = w[I]
              ;((W -= P.measureText(K).width), P.fillText(K, W, x), I > 0 && (W -= z))
            }
          else for (let I = 0; I < w.length; I++) (P.fillText(w[I], W, x), (W += P.measureText(w[I]).width + z))
        },
        C = (P, w) => {
          let v = u + p * w + p / 2 + f / 3
          this.isSafari ? k(t, P, d, v, m, this.textAlign, r) : t.fillText(P, d, v)
        },
        y = this.textContent
          ? this.textContent.split(`
`)
          : [""]
      let O = y.length
      const X = (P, w, v) =>
        w.split("").reduce((z, Y, ie) => ((z += P.measureText(Y).width), ie < w.length - 1 && (z += v), z), 0)
      for (let P = 0; P < O; P++) {
        let w = "",
          v = y[P].split(/(\s|\n)/)
        for (let x = 0; x < v.length; x++) {
          const z = v[x],
            Y = w + z
          if (
            (this.isSafari && m ? X(t, Y, m) : t.measureText(Y).width) > r ||
            z ===
              `
`
          ) {
            if (w !== "")
              ((y[P] = w.trim()),
                x !== v.length - 1
                  ? (y.splice(P + 1, 0, v.slice(x).join("")), O++)
                  : z !==
                      `
` && y.push(z))
            else {
              let B = z,
                W = P
              for (; B.length > 0; ) {
                let I = ""
                for (let K = 0; K < B.length && (t.measureText(I + B[K]).width <= r || K == 0); K++) I += B[K]
                ;((B = B.slice(I.length)), (y[W] = I.trim()), B.length > 0 && (y.splice(W + 1, 0, B), W++, O++))
              }
              v.slice(x + 1).length > 0 && (y[W] += v.slice(x + 1).join(""))
            }
            break
          } else w = Y
          x === v.length - 1 && (y[P] = w.trim())
        }
      }
      ;(y.forEach((P, w) => {
        ;(C(P, c), w < y.length - 1 && c++)
      }),
        t.translate(-(d + r / 2), -(u + l / 2)),
        t.restore(),
        (this.height = p * c + p))
    }
  }
  function $e() {
    document[xe]
      ? (cancelAnimationFrame(ue),
        _.scenes.forEach((n) => {
          n.pauseTime = performance.now()
        }))
      : (Fe(),
        _.scenes.forEach((n) => {
          ;((n.pauseTime = n.pauseTime - performance.now()),
            n.curtain.planes.forEach((e) => {
              e.videos.forEach((t) => {
                t.play()
              })
            }))
        }))
  }
  function Xt() {
    ;((re = window.innerHeight || document.documentElement.clientHeight),
      (J = window.innerWidth || document.documentElement.clientWidth),
      _.scenes
        .filter((n) => n.initialized)
        .forEach((n) => {
          n.refresh()
        }),
      Le())
  }
  function qe(n, e, t = 50) {
    const s = e || n.getBoundingClientRect(),
      i = (s.top >= -t && s.top <= re + t) || (s.bottom >= -t && s.bottom <= re + t) || (s.top <= 0 && s.bottom >= re),
      r = (s.left >= -t && s.left <= J + t) || (s.right >= -t && s.right <= J + t) || (s.left <= 0 && s.right >= J)
    return i && r
  }
  function Le() {
    _.scenes
      .filter((n) => n.getDynamicLayers().length)
      .forEach((n) => {
        let e = n.element.getBoundingClientRect()
        ;((n.scrollY = G),
          n.lastBbox &&
            (Math.abs(e.top - n.lastBbox.top) === 0 && ne > 0
              ? ((n.fixedCounter = (n.fixedCounter || 0) + 1), n.fixedCounter > 3 && (n.isFixed = !0))
              : (n.fixedCounter = 0)),
          (n.lastBbox = n.bbox),
          (n.bbox = e))
      })
  }
  function Qe(n) {
    const e = _.scenes.filter((i) => i.getDynamicLayers().length),
      t = _.scenes.filter((i) => i.rendering)
    e.length && !t.length && Fe()
    const s = performance.now()
    ;((!ce || s - ce > 64) && Le(),
      e.forEach((i) => {
        qe(i.element, i.bbox, 100)
          ? i.isInView ||
            ((i.isInView = !0),
            (i.pauseTime = i.pauseTime - performance.now()),
            i.curtain.resize(),
            i.lazyLoad && !i.initialized && !i.initializing && i.initializePlanes())
          : i.isInView && ((i.isInView = !1), (i.pauseTime = performance.now()))
      }))
  }
  function Yt() {
    _.scenes.forEach((n) => {
      var e, t, s, i
      if (
        n.isInView &&
        (t = (e = n.curtain) == null ? void 0 : e.planes) != null &&
        t.find((r) => r.uniforms.mousePos) &&
        !(Pe() && (i = (s = n.interactivity) == null ? void 0 : s.mouse) != null && i.disableMobile)
      ) {
        ;((n.mouse.pos.y = n.mouse.movePos.y),
          (n.mouse.pos.x = n.mouse.movePos.x),
          (n.mouse.lastPos.x = n.mouse.pos.x),
          (n.mouse.lastPos.y = n.mouse.pos.y))
        let r = n.isFixed ? n.bbox.top : n.bbox.top + n.scrollY,
          a = n.bbox.left
        n.mouse.page.x > a &&
        n.mouse.page.y > r &&
        n.mouse.page.x < n.lastBbox.width + a &&
        n.mouse.page.y < n.lastBbox.height + r
          ? n.mouse.enterTime || (n.mouse.enterTime = performance.now())
          : (n.mouse.enterTime = null)
      }
    })
  }
  function Ze(n) {
    _.scenes
      .filter((e) => e.isInView)
      .forEach((e) => {
        ;((e.mouse.page.x = 99999999999), (e.mouse.page.y = 99999999999), (e.mouse.enterTime = null))
      })
  }
  function Re(n) {
    _.scenes
      .filter((e) => e.isInView && e.initialized)
      .forEach((e) => {
        let t = e.bbox
        const s = performance.now()
        Math.abs(ne) > 1 &&
          s - e.lastBboxTime > 64 &&
          ((t = e.element.getBoundingClientRect()), (e.lastBbox = e.bbox), (e.bbox = t), (e.lastBboxTime = s))
        let i, r
        ;(n.targetTouches
          ? ((i = n.targetTouches[0].pageX), (r = n.targetTouches[0].pageY))
          : ((i = n.pageX), (r = n.pageY)),
          e.isFixed && ((e.scrollY = 0), n.targetTouches ? (r = n.targetTouches[0].clientY) : (r = n.clientY)))
        const a = i - t.left,
          h = r - (t.top + (e.isFixed ? 0 : G))
        ;((e.mouse.page.x = i),
          (e.mouse.page.y = r),
          (e.mouse.movePos.x = a * 0.5),
          (e.mouse.movePos.y = h * 0.5),
          (e.mouse.movedInView = !0))
      })
  }
  _.scenes = []
  class $t {
    constructor(e) {
      L(this, "local", { preloadedTextures: {} })
      L(this, "scrollY", 0)
      L(this, "paused", !1)
      L(this, "lastBboxTime", 0)
      ;((this.id = e.id),
        (this.breakpoints = [
          { name: "Desktop", max: 1 / 0, min: 992 },
          { name: "Tablet", max: 991, min: 576 },
          { name: "Mobile", max: 575, min: 0 },
        ]),
        (this.projectId = e.projectId),
        (this.canvasWidth = e.width || e.element.offsetWidth || J),
        (this.canvasHeight = e.height || e.element.offsetHeight || re),
        (this.curtain = void 0),
        (this.curtainRafId = void 0),
        (this.dpi = +e.dpi || Math.min(1.5, window.devicePixelRatio)),
        (this.element = e.element),
        (this.fps = e.fps || 60),
        (this.name = e.name),
        (this.frameDuration = Math.floor(1e3 / (e.fps || 60))),
        (this.layers = e.layers),
        (this.lazyLoad = e.lazyLoad),
        (this.initialized = !1),
        (this.lasTick = null),
        (this.lastTime = 0),
        (this.rendering = !1),
        (this.bbox = {}),
        (this.isFixed = e.fixed || window.getComputedStyle(this.element).position === "fixed"),
        (this.interactivity = { mouse: { disableMobile: !1 } }),
        (this.mouse = {
          downPos: { x: 0.5, y: 0.5 },
          movePos: { x: 0.5, y: 0.5 },
          lastPos: { x: 0.5, y: 0.5 },
          delta: { x: 0, y: 0 },
          page: { x: 99999999999, y: 99999999999 },
          dragging: !1,
          movedInView: !1,
          enterTime: null,
          ticks: 0,
          pos: { x: 0, y: 0 },
        }),
        (this.renderingScale = e.renderingScale || 1),
        (this.scale = e.scale || 1),
        (this.split = !1),
        (this.versionId = ""),
        e.width &&
          e.height &&
          ((this.element.style.width = e.width + "px"), (this.element.style.height = e.height + "px")),
        (this.bbox = this.element.getBoundingClientRect()),
        (this.lastBbox = this.bbox),
        (this.currentBreakpoint = this.breakpoints.find((t) => J >= t.min && J <= t.max)),
        this.createCurtains(),
        this.setCanvasScale(),
        this.curtain && (this.textureLoader = new Be(this.curtain)))
    }
    preloadTextures() {
      this.layers.forEach((e) => {
        var t, s, i, r, a, h, o, l, d
        if (e.isElement)
          e.local.canvas
            ? this.textureLoader.loadCanvas(
                e.local.canvas,
                { sampler: "uTexture", premultiplyAlpha: !0 },
                (u) => {
                  ;((u.userData.hasStateEffects =
                    e.states.appear.length > 0 || e.states.scroll.length > 0 || e.states.hover.length > 0),
                    (e.preloadedCanvasTexture = u))
                },
                (u) => {
                  console.error("Error loading canvas texture:", u)
                },
              )
            : console.error("Error loading canvas")
        else if (
          ((t = e == null ? void 0 : e.texture) != null && t.src) ||
          ((i = (s = e == null ? void 0 : e.data) == null ? void 0 : s.texture) != null && i.src)
        ) {
          const u =
              ((r = e == null ? void 0 : e.texture) == null ? void 0 : r.src) ||
              ((h = (a = e == null ? void 0 : e.data) == null ? void 0 : a.texture) == null ? void 0 : h.src),
            c =
              ((o = e == null ? void 0 : e.texture) == null ? void 0 : o.sampler) ||
              ((d = (l = e == null ? void 0 : e.data) == null ? void 0 : l.texture) == null ? void 0 : d.sampler) ||
              "uTexture"
          this.local.preloadedTextures[u]
            ? this.local.preloadedTextures[u].count++
            : ((this.local.preloadedTextures[u] = { count: 1, texture: null }),
              this.textureLoader.loadImage(u, { sampler: c, premultipliedAlpha: !1 }, (f) => {
                this.local.preloadedTextures[u].texture = f
              }))
        }
      })
    }
    setCanvasScale() {
      ;((this.canvasWidth = this.element.offsetWidth), (this.canvasHeight = this.element.offsetHeight))
    }
    destroy() {
      var e, t
      ;((e = this.curtain) != null && e.gl && (this.curtain.dispose(), (this.curtain = null)),
        this.element &&
          (this.element.removeAttribute("data-us-initialized"),
          this.element.removeAttribute("data-scene-id"),
          (t = this.element.querySelector("canvas")) == null || t.remove(),
          this.element.querySelectorAll("[data-us-text]").forEach((s) => {
            s.remove()
          })),
        this.layers.filter((s) => s.dispose).forEach((s) => s.dispose()),
        (this.layers = []),
        (_.scenes = _.scenes.filter((s) => s.id !== this.id)),
        _.scenes.length || De())
    }
    resize() {
      ;(this.setCanvasScale(),
        this.layers
          .filter((e) => e.isElement)
          .forEach((e) => {
            e.resize()
          }),
        (this.lastBbox = this.bbox),
        (this.bbox = this.element.getBoundingClientRect()),
        (this.currentBreakpoint = this.breakpoints.find((e) => J >= e.min && J <= e.max)),
        this.curtain &&
          (this.curtain.resize(),
          this.curtain.planes.forEach((e) => {
            e.uniforms.resolution &&
              ((e.uniforms.resolution.value.x = this.curtain.canvas.width),
              (e.uniforms.resolution.value.y = this.curtain.canvas.height))
          })))
    }
    refresh() {
      ;((this.initialized = !1),
        this.layers.forEach((e) => {
          var t, s
          ;((t = e.states) == null || t.scroll.forEach((i) => i.resetState()),
            (s = e.states) == null ||
              s.appear.forEach((i) => {
                e[i.prop] = i.endValue ?? e[i.prop]
              }),
            e.breakpoints.length &&
              (e.setBreakpointValues(),
              e.local.bpProps &&
                Object.keys(e.local.bpProps).forEach((i) => {
                  e.getPlanes().forEach((r) => {
                    r.uniforms[i] && Te(e.local.bpProps[i], r.uniforms[i], i)
                  })
                })),
            e.preloadedCanvasTexture && (e.preloadedCanvasTexture.shouldUpdate = !0))
        }),
        requestAnimationFrame(() => {
          this.curtain &&
            (this.curtain.planes.forEach((e) => {
              e._canDraw = !0
            }),
            (this.scrollY = G),
            this.resize(),
            this.handlePlaneCreation())
        }))
    }
    getDynamicLayers() {
      return this.layers.filter((e) => oe(e))
    }
    createCurtains() {
      if (
        ((this.curtain = new at({
          container: this.element,
          premultipliedAlpha: !0,
          antialias: !1,
          autoRender: !1,
          autoResize: !1,
          watchScroll: !1,
          renderingScale: Math.min(Math.max(0.25, this.renderingScale), 1),
          production: !1,
          pixelRatio: this.dpi,
        })),
        !this.curtain.gl)
      ) {
        console.log("Unicorn.studio was unable to create WebGL context")
        return
      }
      ;(this.curtain.onError((e) => {
        console.warn("Unicorn.studio scene encountered an error", e)
      }),
        this.curtain.onContextLost(() => {
          ;(console.warn("Unicorn.studio scene lost WebGL context"), this.curtain.restoreContext())
        }),
        (this.scrollY = G))
    }
    renderFrame() {
      Ot(this.curtain)
        ? (this.layers.forEach((e) => {
            e.local.currentFrameMouseValues = null
          }),
          this.handleStateEffects(),
          this.curtain.render())
        : console.warn("Curtains instance is not valid")
    }
    renderNFrames(e, t) {
      let s = 0
      const i = () => {
        ;(this.renderFrame(), s < e ? (s++, requestAnimationFrame(i)) : t && t())
      }
      this.rendering || i()
    }
    setInteractiveParams(e, t) {
      let s = { mouse: { disableMobile: !1 } }
      ;(t && t.mouse && "disableMobile" in t.mouse && (s.mouse.disableMobile = t.mouse.disableMobile),
        e &&
          e.interactivity &&
          e.interactivity.mouse &&
          "disableMobile" in e.interactivity.mouse &&
          (s.mouse.disableMobile = e.interactivity.mouse.disableMobile),
        (this.interactivity = s))
    }
    getSplitOrderedItems() {
      let e = this.getOrderedItems(),
        t = 0,
        s = e[t]
      if (s) {
        let i = s.parentLayer ? s.getParent() : null,
          r = i && oe(i),
          a = i && i.effects && i.effects.length && i.getChildEffectItems().filter((h) => oe(h)).length
        for (; s && !oe(s) && !r && !a; )
          (t++,
            (s = e[t]),
            s &&
              ((i = s.parentLayer ? s.getParent() : null),
              (r = i && oe(i)),
              (a = i && i.effects && i.effects.length && i.getChildEffectItems().filter((h) => oe(h)).length)))
        return { static: this.getOrderedItems().splice(0, t), dynamic: this.getOrderedItems().splice(t) }
      } else return { static: [], dynamic: [] }
    }
    initializePlanes(e) {
      ;((this.initializing = !0),
        this.handleItemPlanes(() => {
          ;(this.handlePlaneCreation(), e && e(this))
        }))
    }
    getPassPlane(e, t) {
      var s
      return (s = this.curtain) == null
        ? void 0
        : s.planes.find((i) => i.userData.id === e.local.id && i.userData.passIndex === t)
    }
    getRenderTargets() {
      var e
      return (e = this.curtain) == null ? void 0 : e.renderTargets.filter((t) => t.userData.id)
    }
    getPlanes() {
      var e
      return (e = this.curtain) == null ? void 0 : e.planes.filter((t) => t.type !== "PingPongPlane")
    }
    getPlaneParams(e, t) {
      var h, o, l, d, u
      let s =
        ((h = e.data) != null && h.heightSegments) ||
        ((o = e.data) != null && o.widthSegments) ||
        ["noiseField", "bulge"].includes(e.type)
          ? 500
          : 1
      const i = {
        resolution: { name: "uResolution", type: "2f", value: new A(this.canvasWidth, this.canvasHeight) },
        mousePos: { name: "uMousePos", type: "2f", value: new A(0.5) },
        time: { name: "uTime", type: "1f", value: 0 },
        dpi: { name: "uDpi", type: "1f", value: this.dpi * +this.renderingScale },
      }
      ;(e.isElement && (i.sampleBg = { name: "uSampleBg", type: "1i", value: 1 }),
        e.usesPingPong && (i.previousMousePos = { name: "uPreviousMousePos", type: "2f", value: new A(0.5) }))
      let r = e.compiledFragmentShaders[t] || e.compiledFragmentShaders[0],
        a = e.compiledVertexShaders[t] || e.compiledVertexShaders[0]
      if (
        (e.states &&
          [...e.states.appear, ...e.states.scroll, ...e.states.hover].forEach((c) => {
            i[c.prop] || (c.uniformData && ((i[c.prop] = c.uniformData), (i[c.prop].value = e[c.prop])))
          }),
        (l = e.data) != null && l.uniforms)
      )
        for (let c in e.data.uniforms) {
          let f = e.data.uniforms[c]
          ;((i[c] = e.data.uniforms[c]),
            ((d = f.value) == null ? void 0 : d.type) === "Vec3"
              ? (i[c].value = new b(f.value._x, f.value._y, f.value._z))
              : ((u = f.value) == null ? void 0 : u.type) === "Vec2"
                ? (i[c].value = new A(f.value._x, f.value._y))
                : typeof f.value == "object" && (i[c].value = ee(f.value)))
        }
      return {
        fragmentShader: r,
        vertexShader: a,
        widthSegments: s,
        heightSegments: s,
        texturesOptions: { floatingPoint: "half-float", premultiplyAlpha: !0 },
        uniforms: i,
      }
    }
    createPlane(e, t, s) {
      var a
      let i
      ;(e.isElement ? (i = this.getPlaneParams(e)) : (i = this.getPlaneParams(e, s ? s.index : null)),
        (i.watchScroll = !1))
      let r
      try {
        if (!this.curtain.container) throw new Error("Can't find scene container")
        let h =
            (s == null ? void 0 : s.downSample) ||
            ((s == null ? void 0 : s.index) === 0 && ((a = e.data) == null ? void 0 : a.downSample)),
          o = this.renderingScale
        if (
          (h && ((o = typeof h == "number" ? h : 0.5), (o *= this.renderingScale)),
          e.userDownsample && typeof e.userDownsample == "number" && (o *= e.userDownsample),
          (r = new He(this.curtain, this.curtain.container, i)),
          !r || !r.userData || !r.textures)
        )
          throw new Error("Plane not properly initialized")
        return (
          (r.textures.length = 0),
          (r.userData.id = e.local.id),
          (r.userData.layerType = e.layerType),
          (r.userData.type = e.type),
          (r.userData.downSample = o),
          r.setRenderOrder(t),
          r
        )
      } catch (h) {
        return (console.error("Error creating plane:", h), null)
      }
    }
    createPingPongPlane(e, t, s) {
      var a
      let i = this.getPlaneParams(e, 1 + ((s == null ? void 0 : s.length) || 0)),
        r =
          (a = this.curtain) == null
            ? void 0
            : a.planes.find((h) => h.type === "PingPongPlane" && h.userData.id === e.local.id)
      if (r) r.setRenderOrder(t)
      else {
        if (((r = new Ft(this.curtain, this.curtain.container, i)), !r)) return
        ;((r.userData.id = e.local.id),
          (r.userData.pingpong = !0),
          r.setRenderOrder(t),
          r
            .onReady(() => {
              ;(this.setInitialEffectPlaneUniforms(r, e, e.getParent(), s), (r.userData.isReady = !0))
            })
            .onRender(() => {
              this.setEffectPlaneUniforms(r, e)
            }))
      }
      if (r) return r
    }
    createEffectPlane(e, t, s) {
      const i = this.createPlane(e, t, s)
      if (!i) return
      const r = e.getParent()
      if (!i || !i.userData || !i.textures) throw new Error("Plane not properly initialized", i)
      ;(s &&
        ((i.userData.passIndex = s.index),
        (i.userData.length = e.data.passes.length),
        Object.entries(s).forEach(([a, h]) => {
          i.uniforms[a] && (i.uniforms[a].value = h)
        })),
        this.setInitialEffectPlaneUniforms(i, e, r, s),
        i
          .onReady(() => {
            i.userData.isReady = !0
          })
          .onRender(() => this.setEffectPlaneUniforms(i, e)))
    }
    createElementPlane(e, t) {
      const s = this.createPlane(e, t)
      s &&
        s
          .onReady(() => {
            ;(this.setInitialElementPlaneUniforms(s, e), (s.userData.isReady = !0))
          })
          .onRender(() => this.setElementPlaneUniforms(s, e))
    }
    handleMediaTextures(e, t, s) {
      var i
      if (
        ([t.texture, (i = t.data) == null ? void 0 : i.texture]
          .filter((r) => (r == null ? void 0 : r.src))
          .forEach((r) => {
            var h
            if ((h = t.compiledFragmentShaders[s.passIndex ?? 0]) == null ? void 0 : h.includes(r.sampler)) {
              const o = this.local.preloadedTextures[r.src]
              o != null && o.texture
                ? (o.count > 1 ? (j(e, r.sampler, o.texture), o.count--) : e.addTexture(o.texture),
                  (e.userData.textureLoaded = !0))
                : this.textureLoader.loadImage(r.src, { premultipliedAlpha: !1, sampler: r.sampler }, (l) => {
                    ;(e.addTexture(l), (e.userData.textureLoaded = !0))
                  })
            }
          }),
        t.type === "video")
      ) {
        const r = t.src.includes(", ") ? Dt(t.src) : t.src
        e.loadVideo(r, { premultipliedAlpha: !0, sampler: "uVideoTexture" }, (a) => {
          ;(e.videos.at(-1).play(), (e.userData.textureLoaded = !0))
        })
      }
    }
    handleEffectPlane(e, t, s) {
      var o
      const i = "passIndex" in s ? this.getPassPlane(e, s.passIndex) : e.getPlane()
      if (!i) return !1
      let r = this.getRenderTargets()[t - 1],
        a =
          (o = this.curtain) == null
            ? void 0
            : o.planes.find((l) => l.type === "PingPongPlane" && l.userData.id === e.local.id),
        h = e.compiledFragmentShaders[s.passIndex ?? 0]
      if (
        (a && h != null && h.includes("uPingPongTexture") && j(i, "uPingPongTexture", a.getTexture()),
        r ? j(i, "uTexture", r.getTexture()) : ze(i, this.canvasWidth, this.canvasHeight),
        h != null && h.includes("uBgTexture"))
      ) {
        let l = this.getRenderTargets()[t - (1 + s.passIndex)]
        l && j(i, "uBgTexture", l.getTexture())
      }
      this.handleMediaTextures(i, e, s)
    }
    handleElementPlane(e, t) {
      const s = e.getPlane()
      if (!s) return !1
      const i = e.getChildEffectItems(),
        r = this.layers.filter((l) => !l.parentLayer)
      let a = this.getRenderTargets()[t - 1],
        h = r[r.indexOf(e) - 2],
        o
      if (
        (e.mask && h && (o = h.local.lastTarget),
        i.length || (s.textures.length = 0),
        a || ze(s, this.canvasWidth, this.canvasHeight),
        a && i.length ? j(s, "uTexture", a.getTexture()) : s && s.addTexture(e.preloadedCanvasTexture),
        a)
      ) {
        if (i.length) {
          let l = i.reduce((d, u) => d + u.getPlanes().length, 0)
          a = this.getRenderTargets()[t - (1 + l)]
        }
        a && (j(s, "uBgTexture", a.getTexture()), o && e.mask && j(s, "uPreviousLayerTexture", o.getTexture()))
      }
    }
    handleChildEffectPlane(e, t, s) {
      var f
      const i = "passIndex" in s ? this.getPassPlane(e, s.passIndex) : e.getPlane()
      if (!i) return
      const r = e.getParent()
      let a = this.getRenderTargets()[t - 1],
        h =
          (f = this.curtain) == null
            ? void 0
            : f.planes.find((p) => p.type === "PingPongPlane" && p.userData.id === e.local.id),
        o = e.compiledFragmentShaders[s.passIndex ?? 0],
        l = r.effects.filter((p) => {
          if (this.layers.find((m) => m.parentLayer === p)) return this.layers.find((m) => m.parentLayer === p)
        }),
        d = l.indexOf(e.parentLayer),
        u = l[l.length - 1] === l[d],
        c = s.passIndex === s.length
      if (
        (a && (d || s.passIndex > 0)
          ? (e.isMask && (!s.length || (u && c)) && i.addTexture(r.preloadedCanvasTexture),
            j(i, "uTexture", a.getTexture()))
          : e.isMask
            ? (u && c && i.addTexture(r.preloadedCanvasTexture),
              a ? j(i, "uTexture", a.getTexture()) : ze(i, this.canvasWidth, this.canvasHeight))
            : o != null &&
              o.includes("uTexture") &&
              (i.addTexture(r.preloadedCanvasTexture), (r.preloadedCanvasTexture.userData.used = !0)),
        h && o != null && o.includes("uPingPongTexture") && j(i, "uPingPongTexture", h.getTexture()),
        o != null && o.includes("uBgTexture"))
      )
        if (d && s.length > 1) {
          let p = this.getRenderTargets()[t - (1 + s.length)]
          p && j(i, "uBgTexture", p.getTexture())
        } else
          r.preloadedCanvasTexture.userData.used
            ? i.loadCanvas(r.local.canvas, { sampler: "uBgTexture", premultiplyAlpha: !0 }, (p) => {
                p.shouldUpdate = r.preloadedCanvasTexture.shouldUpdate
              })
            : ((r.preloadedCanvasTexture._samplerName = "uBgTexture"), i.addTexture(r.preloadedCanvasTexture))
      ;(e.type === "custom" && j(i, "uCustomTexture", this.getRenderTargets()[t]), this.handleMediaTextures(i, e, s))
    }
    createPlanes() {
      this.getOrderedItems().forEach((e, t) => {
        e.getPlanes().length
          ? e.getPlanes().forEach((s) => s.setRenderOrder(t))
          : e.isElement
            ? this.createElementPlane(e, t)
            : this.createEffectPlanes(e, t)
      })
    }
    createEffectPlanes(e, t) {
      const s = e.data
      s.passes && s.passes.length
        ? (this.createEffectPlane(e, t, { index: 0, downSample: s.downSample, length: s.passes.length + 1 }),
          s.passes.forEach((i, r) => {
            this.createEffectPlane(e, t, {
              index: r + 1,
              length: s.passes.length + 1,
              downSample: i.downSample,
              [i.prop]: i.value,
            })
          }),
          e.usesPingPong && this.createPingPongPlane(e, t, s.passes))
        : (this.createEffectPlane(e, t), e.usesPingPong && this.createPingPongPlane(e, t))
    }
    createTextures() {
      const e = this.getPlanes().sort((s, i) => s.renderOrder - i.renderOrder),
        t = e.length
      for (let s = 0; s < t; s++) {
        const i = e[s]
        let r = this.layers.find((a) => a.local.id === i.userData.id)
        ;(s < t - 1 && this.assignRenderTargetToPlane(e, s, r, i),
          this.handleTextures(r, s, i.userData),
          (r.local.lastTarget = i.target))
      }
    }
    assignRenderTargetToPlane(e, t, s, i) {
      let r = this.getTextureParams(e, t, s),
        a = this.getRenderTargets()[t] || new Me(this.curtain, r)
      ;((a.userData.id = i.userData.id), i.setRenderTarget(a))
    }
    handleTextures(e, t, s) {
      e.isElement
        ? this.handleElementPlane(e, t)
        : e.parentLayer
          ? this.handleChildEffectPlane(e, t, s)
          : this.handleEffectPlane(e, t, s)
    }
    handleItemPlanes(e) {
      ;(this.createPlanes(), this.createTextures(), this.checkIfReady(e))
    }
    checkIfReady(e) {
      const t = () => {
        var r
        let s = !1
        const i = (r = this.curtain) == null ? void 0 : r.planes
        if (i) {
          for (let a = 0; a < i.length; a++)
            if (!i[a].userData.isReady) {
              s = !0
              break
            }
          s ? (this.renderFrame(), requestAnimationFrame(t)) : e()
        }
      }
      t()
    }
    setInitialEffectPlaneUniforms(e, t, s, i) {
      var r
      if (!e.userData.initialUniformsSet || !e.userData.isReady) {
        for (let a in e.uniforms)
          ((r = t.states) != null && r.scroll.some((h) => h.prop === a)) ||
            (t.local.bpProps && a in t.local.bpProps
              ? Te(t.local.bpProps[a], e.uniforms[a], a)
              : a in t && Te(t[a], e.uniforms[a], a))
        ;(s && i && i.index < i.length - 1 && e.uniforms.isMask && (e.uniforms.isMask.value = 0),
          s && t.isMask && !t.mouseMomentum && (t.mouseMomentum = s.mouseMomentum),
          (e.uniforms.resolution.value.x = this.curtain.canvas.width),
          (e.uniforms.resolution.value.y = this.curtain.canvas.height),
          (e.userData.initialUniformsSet = !0))
      }
    }
    handleStateEffects() {
      this.layers.forEach((e) => {
        e.updateStateEffects()
      })
    }
    setInitialElementPlaneUniforms(e, t) {
      var s
      ;((e.uniforms.resolution.value.x = this.curtain.canvas.width),
        (e.uniforms.resolution.value.y = this.curtain.canvas.height))
      for (let i in e.uniforms)
        t.local.bpProps && i in t.local.bpProps
          ? (e.uniforms[i].value = t.local.bpProps[i])
          : i in t && (e.uniforms[i].value = ((s = t.local.stateEffectProps) == null ? void 0 : s[i]) ?? t[i])
      e.uniforms.sampleBg &&
        (e.renderOrder - t.effects.length === 0 ? (e.uniforms.sampleBg.value = 0) : (e.uniforms.sampleBg.value = 1))
    }
    setElementPlaneUniforms(e, t) {
      if (e.uniforms.mousePos) {
        t.local.currentFrameMouseValues || (t.local.currentFrameMouseValues = this.calculateItemMouseValues(t))
        const s = t.local.currentFrameMouseValues
        if (!s) return
        this.mouse.movedInView &&
          (e.uniforms.mousePos.value.x !== s.x || e.uniforms.mousePos.value.y !== s.y) &&
          ((e.uniforms.mousePos.value.x = s.x), (e.uniforms.mousePos.value.y = s.y))
      }
    }
    calculateItemMouseValues(e) {
      if (!this.mouse.movedInView) return null
      const t = this.bbox.width / 2,
        s = this.bbox.height / 2,
        i = this.getMousePosWithMomentum(e, t, s)
      return (
        this.local.mouseValuePool || (this.local.mouseValuePool = new A(0.5)),
        (this.local.mouseValuePool.x = i.x),
        (this.local.mouseValuePool.y = i.y),
        this.local.mouseValuePool
      )
    }
    getMousePosWithMomentum(e, t, s) {
      let i = this.mouse.pos.x,
        r = this.mouse.pos.y
      const a = e.mouseMomentum || 0
      if (!a) return { x: i / t, y: 1 - r / s }
      if (!e.local.lastMousePos) return ((e.local.lastMousePos = { x: i / t, y: 1 - r / s }), e.local.lastMousePos)
      const h = e.local.lastMousePos.x * t,
        o = (1 - e.local.lastMousePos.y) * s,
        l = Math.abs(i - h),
        d = Math.abs(r - o),
        u = 0.001
      return (
        (l < u && d < u) ||
          ((i = be(i, h, a * 2)),
          (r = be(r, o, a * 2)),
          (e.local.lastMousePos.x = i / t),
          (e.local.lastMousePos.y = 1 - r / s)),
        e.local.lastMousePos
      )
    }
    setEffectPlaneUniforms(e, t) {
      var r, a, h
      let s = ((r = t.local.stateEffectProps) == null ? void 0 : r.speed) ?? t.speed ?? 1
      if (
        (t.animating && e.uniforms.time && (e.uniforms.time.value += (s * 60) / this.fps),
        Pe() && (h = (a = this.interactivity) == null ? void 0 : a.mouse) != null && h.disableMobile)
      )
        return !1
      if (!this.mouse.movedInView) return
      t.local.currentFrameMouseValues || (t.local.currentFrameMouseValues = this.calculateItemMouseValues(t))
      const i = t.local.currentFrameMouseValues
      i &&
        (e.uniforms.previousMousePos &&
          (this.mouse.ticks > 16
            ? ((e.uniforms.previousMousePos.value.x = e.uniforms.mousePos.value.x),
              (e.uniforms.previousMousePos.value.y = e.uniforms.mousePos.value.y))
            : ((e.uniforms.previousMousePos.value.x = i.x), (e.uniforms.previousMousePos.value.y = i.y))),
        e.uniforms.mousePos &&
          !i.equals(e.uniforms.mousePos.value) &&
          ((e.uniforms.mousePos.value.x = i.x), (e.uniforms.mousePos.value.y = i.y)),
        this.mouse.ticks++)
    }
    getOrderedItems() {
      let e = []
      return (
        this.layers
          .filter((t) => !t.parentLayer)
          .forEach((t) => {
            ;(t.effects && t.effects.length && e.push(...t.getChildEffectItems()), e.push(t))
          }),
        e
      )
    }
    getTextureParams(e, t, s) {
      var h
      const i = e[t]
      let r = 1
      return (
        i.userData.downSample && (r = typeof i.userData.downSample == "number" ? i.userData.downSample : 0.5),
        {
          maxWidth: this.curtain.canvas.width * r,
          maxHeight: this.curtain.canvas.height * r,
          depth: (h = s == null ? void 0 : s.data) == null ? void 0 : h.depth,
        }
      )
    }
    handlePlaneCreation() {
      ;((this.initialized = !0),
        (this.initializing = !1),
        this.rendering || this.renderNFrames(2),
        this.waitForAssetsAndDisablePlanes(),
        Fe())
    }
    waitForAssetsAndDisablePlanes() {
      this.layers.some(
        (t) =>
          ((t.layerType === "image" || t.layerType === "text") && !t.local.loaded) ||
          Object.values(this.local.preloadedTextures).some((s) => s === null),
      )
        ? setTimeout(() => this.waitForAssetsAndDisablePlanes(), 64)
        : (this.layers
            .filter((t) => t.preloadedCanvasTexture)
            .forEach((t) => {
              t.preloadedCanvasTexture.shouldUpdate = t.preloadedCanvasTexture.userData.hasStateEffects || !1
            }),
          setTimeout(() => this.disablePlanes(), 16))
    }
    disablePlanes() {
      const e = this.getSplitOrderedItems()
      e.dynamic.length || e.static.pop()
      for (const t of e.static)
        t.getPlanes().forEach((s) => {
          s._canDraw = !1
        })
    }
  }
  function qt(n) {
    return typeof HTMLElement == "object"
      ? n instanceof HTMLElement
      : n && typeof n == "object" && n !== null && n.nodeType === 1 && typeof n.nodeName == "string"
  }
  function te(n, e, t) {
    n.addEventListener(e, t, { passive: !0 })
  }
  function se(n, e, t) {
    n.removeEventListener(e, t)
  }
  function Qt() {
    ;(te(window, "scroll", Qe),
      Pe() ? te(window, "touchmove", Re) : te(window, "mousemove", Re),
      te(window, "routeChange", Xe),
      te(document, "mouseleave", Ze),
      (he = zt(Xt, 16)),
      Pe() ? te(window, "orientationchange", he) : te(window, "resize", he),
      te(document, de, $e),
      (Ce = !0))
  }
  function De() {
    ;(se(window, "mousemove", Re),
      se(window, "touchmove", Re),
      se(window, "scroll", Qe),
      se(window, "routeChange", Xe),
      se(document, "mouseleave", Ze),
      he && (se(window, "resize", he), se(window, "orientationchange", he)),
      se(document, de, $e),
      (Ce = !1))
  }
  function Zt(n, e, t) {
    return { canvasWidth: n.offsetWidth, canvasHeight: n.offsetHeight, scale: e, dpi: t, element: n }
  }
  function Jt() {
    ;(_.scenes.forEach((n) => {
      n.destroy()
    }),
      (_.scenes.length = 0),
      De())
  }
  function Kt(n, e, t, s, i) {
    let r
    if (t) {
      r = t
      const a = document.getElementById(t)
      if (a)
        try {
          let h = JSON.parse(a.innerText)
          return h != null && h.options && h != null && h.history
            ? Promise.resolve(h)
            : (s(new Error(`Did not find valid JSON inside ${t}`)), Promise.reject())
        } catch (h) {
          return (s(new Error(`Error parsing JSON from ${t}: ${h.message}`)), Promise.reject())
        }
    } else {
      let a = "https://storage.googleapis.com/unicornstudio-production"
      ;(i || (e != null && e.includes("production=true"))
        ? ((a = "https://assets.unicorn.studio"), (e = `v=${Date.now()}`))
        : (e != null && e.includes("update=")) || (e = `v=${Date.now()}`),
        (r = `${a}/embeds/${n}${e ? "?" + e : ""}`))
    }
    return fetch(r)
      .then((a) => a.json())
      .then((a) => a)
      .catch((a) => console.error("Error fetching data:", a))
  }
  function es() {
    try {
      return !!document.createElement("canvas").getContext("webgl2")
    } catch {
      return !1
    }
  }
  function Je(n) {
    let e = n.projectId ? n.projectId.split("?")[0] : null,
      t = n.projectId ? n.projectId.split("?")[1] : null
    return new Promise((s, i) => {
      Kt(e, t, n.filePath, i, n.production)
        .then((r) => {
          ;(!r || !r.history || !r.options) && i(new Error(`Error fetching data for project id '${n.projectId}'`))
          const a = r.options || {},
            h = qt(n.element) ? n.element : document.getElementById(n.elementId)
          if (!h) {
            i(new Error(`Couldn't find an element with id '${n.elementId}' on the page.`))
            return
          }
          const o = pe()
          h.setAttribute("data-scene-id", o)
          let l = n.scale || a.scale || 1,
            d = n.dpi || a.dpi || Math.min(1.5, window.devicePixelRatio)
          const u = It(r.history, o, Zt(h, l, d)),
            c = new $t({
              id: o,
              fps: n.fps || a.fps || 60,
              dpi: d,
              name: a.name,
              projectId: e || n.filePath.split(".")[0],
              renderingScale: l,
              element: h,
              lazyLoad: n.lazyLoad,
              fixed: n.fixed,
              width: n.width,
              height: n.height,
            })
          ;((c.curtain.canvas.innerText = n.altText || ""),
            c.curtain.canvas.setAttribute("aria-label", n.ariaLabel || "Unicorn Studio Scene"),
            c.curtain.canvas.setAttribute("role", "img"))
          let f = a[fe("ZnJlZVBsYW4=")]
          ;((f || a[fe("aW5jbHVkZUxvZ28=")]) && Lt(o, h),
            f && console.log(fe("TWFkZSB3aXRoIHVuaWNvcm4uc3R1ZGlv")),
            _.scenes.push(c),
            (c.layers = u),
            c.preloadTextures(),
            c.setInteractiveParams(n, a),
            (c.isInView = c.isFixed || qe(c.element, c.bbox, 50)),
            (!c.lazyLoad || c.isInView) && c.initializePlanes(),
            Ce || Qt(),
            s(c))
        })
        .catch((r) => {
          ;(console.error(r), i(r))
        })
    })
  }
  function ts() {
    return es()
      ? new Promise((n, e) => {
          const t = [...document.querySelectorAll("[data-us-project], [data-us-project-src]")]
          t.filter((s) => !s.getAttribute("data-us-initialized")).forEach((s, i) => {
            const r = $(s, "data-us-project"),
              a = $(s, "data-us-project-src"),
              h = $(s, "data-us-dpi"),
              o = $(s, "data-us-scale"),
              l = $(s, "data-us-lazyload"),
              d = $(s, "data-us-production"),
              u = $(s, "data-us-fps"),
              c = $(s, "data-us-alttext"),
              f = $(s, "data-us-arialabel"),
              p = $(s, "data-us-disablemobile"),
              m = $(s, "data-us-fixed")
            ;(s.setAttribute("data-us-initialized", !0),
              Je({
                projectId: a ? null : r,
                filePath: a,
                element: s,
                dpi: +h,
                scale: +o,
                production: d,
                fps: +u,
                lazyLoad: l,
                altText: c,
                ariaLabel: f,
                fixed: m,
                interactivity: p ? { mouse: { disableMobile: !0 } } : null,
              }).then((T) => {
                i === t.length - 1 && n(_.scenes)
              }))
          })
        })
      : (console.warn("WebGL2 is not supported in this browser. Unicorn Studio initialization skipped."),
        Promise.resolve([]))
  }
  ;((_.addScene = Je),
    (_.destroy = Jt),
    (_.init = ts),
    (_.unbindEvents = De),
    Object.defineProperty(_, Symbol.toStringTag, { value: "Module" }))
})
