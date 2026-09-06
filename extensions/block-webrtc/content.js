(function () {
  "use strict";

  function blocked(name) {
    return function () {
      throw new DOMException(name + " is disabled by Identity Keeper", "SecurityError");
    };
  }

  function replace(target, name, value) {
    try {
      Object.defineProperty(target, name, {
        configurable: false,
        enumerable: false,
        writable: false,
        value: value
      });
    } catch (_) {
      try {
        target[name] = value;
      } catch (_) {}
    }
  }

  replace(window, "RTCPeerConnection", blocked("RTCPeerConnection"));
  replace(window, "webkitRTCPeerConnection", blocked("webkitRTCPeerConnection"));
  replace(window, "RTCDataChannel", blocked("RTCDataChannel"));

  if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) {
    replace(navigator.mediaDevices, "getUserMedia", function () {
      return Promise.reject(new DOMException("getUserMedia is disabled by Identity Keeper", "SecurityError"));
    });
  }
})();
