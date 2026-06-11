//go:build wgc

#include <unknwn.h>
#include <windows.h>
#include <d3d11.h>
#include <d3dcompiler.h>
#include <dxgi.h>
#include <cstring>
#include <mutex>

#include <winrt/base.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Graphics.Capture.h>
#include <winrt/Windows.Graphics.DirectX.h>
#include <winrt/Windows.Graphics.DirectX.Direct3D11.h>
#include <windows.graphics.capture.interop.h>
#include <windows.graphics.directx.direct3d11.interop.h>

using namespace winrt;
using namespace Windows::Graphics;
using namespace Windows::Graphics::Capture;
using namespace Windows::Graphics::DirectX;
using namespace Windows::Graphics::DirectX::Direct3D11;

extern "C" HRESULT __stdcall CreateDirect3D11DeviceFromDXGIDevice(
  IDXGIDevice* dxgiDevice,
  IInspectable** graphicsDevice);

namespace {

struct PreviewState {
  std::mutex mu;
  HWND parent = nullptr;
  HWND hwnd = nullptr;
  HWND captureHwnd = nullptr;
  int x = 0;
  int y = 0;
  int width = 0;
  int height = 0;
  bool running = false;

  com_ptr<ID3D11Device> device;
  com_ptr<ID3D11DeviceContext> context;
  com_ptr<IDXGISwapChain> swapChain;
  com_ptr<ID3D11VertexShader> vertexShader;
  com_ptr<ID3D11PixelShader> pixelShader;
  com_ptr<ID3D11SamplerState> sampler;
  com_ptr<ID3D11Texture2D> shaderTexture;
  com_ptr<ID3D11ShaderResourceView> shaderView;

  IDirect3DDevice winrtDevice{ nullptr };
  GraphicsCaptureItem item{ nullptr };
  Direct3D11CaptureFramePool framePool{ nullptr };
  GraphicsCaptureSession session{ nullptr };
  event_token frameToken{};
  SizeInt32 lastItemSize{};
};

PreviewState g;

constexpr wchar_t kPreviewClass[] = L"BitStreamWGCPreviewWindow";

LRESULT CALLBACK PreviewWndProc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
  if (msg == WM_ERASEBKGND) {
    return 1;
  }
  return DefWindowProcW(hwnd, msg, wp, lp);
}

bool ensureWindowClass() {
  static bool registered = false;
  if (registered) {
    return true;
  }
  WNDCLASSEXW wc{};
  wc.cbSize = sizeof(wc);
  wc.lpfnWndProc = PreviewWndProc;
  wc.hInstance = GetModuleHandleW(nullptr);
  wc.hCursor = LoadCursorW(nullptr, IDC_ARROW);
  wc.lpszClassName = kPreviewClass;
  registered = RegisterClassExW(&wc) != 0 || GetLastError() == ERROR_CLASS_ALREADY_EXISTS;
  return registered;
}

bool createShaders() {
  static const char* shader = R"(
Texture2D frameTex : register(t0);
SamplerState frameSampler : register(s0);

struct VSOut {
  float4 pos : SV_POSITION;
  float2 uv : TEXCOORD0;
};

VSOut vsMain(uint id : SV_VertexID) {
  float2 pos[4] = {
    float2(-1.0, -1.0),
    float2(-1.0,  1.0),
    float2( 1.0, -1.0),
    float2( 1.0,  1.0)
  };
  float2 uv[4] = {
    float2(0.0, 1.0),
    float2(0.0, 0.0),
    float2(1.0, 1.0),
    float2(1.0, 0.0)
  };
  VSOut output;
  output.pos = float4(pos[id], 0.0, 1.0);
  output.uv = uv[id];
  return output;
}

float4 psMain(VSOut input) : SV_TARGET {
  return frameTex.Sample(frameSampler, input.uv);
}
)";

  com_ptr<ID3DBlob> vsBlob;
  com_ptr<ID3DBlob> psBlob;
  com_ptr<ID3DBlob> errors;
  UINT flags = D3DCOMPILE_ENABLE_STRICTNESS;
  HRESULT hr = D3DCompile(shader, strlen(shader), nullptr, nullptr, nullptr, "vsMain", "vs_5_0", flags, 0, vsBlob.put(), errors.put());
  if (FAILED(hr)) {
    return false;
  }
  errors = nullptr;
  hr = D3DCompile(shader, strlen(shader), nullptr, nullptr, nullptr, "psMain", "ps_5_0", flags, 0, psBlob.put(), errors.put());
  if (FAILED(hr)) {
    return false;
  }
  hr = g.device->CreateVertexShader(vsBlob->GetBufferPointer(), vsBlob->GetBufferSize(), nullptr, g.vertexShader.put());
  if (FAILED(hr)) {
    return false;
  }
  hr = g.device->CreatePixelShader(psBlob->GetBufferPointer(), psBlob->GetBufferSize(), nullptr, g.pixelShader.put());
  if (FAILED(hr)) {
    return false;
  }

  D3D11_SAMPLER_DESC sampler{};
  sampler.Filter = D3D11_FILTER_MIN_MAG_MIP_LINEAR;
  sampler.AddressU = D3D11_TEXTURE_ADDRESS_CLAMP;
  sampler.AddressV = D3D11_TEXTURE_ADDRESS_CLAMP;
  sampler.AddressW = D3D11_TEXTURE_ADDRESS_CLAMP;
  sampler.MaxLOD = D3D11_FLOAT32_MAX;
  return SUCCEEDED(g.device->CreateSamplerState(&sampler, g.sampler.put()));
}

bool createDevice() {
  if (g.device) {
    return true;
  }

  UINT flags = D3D11_CREATE_DEVICE_BGRA_SUPPORT;
  D3D_FEATURE_LEVEL levels[] = {
    D3D_FEATURE_LEVEL_11_1,
    D3D_FEATURE_LEVEL_11_0,
    D3D_FEATURE_LEVEL_10_1,
    D3D_FEATURE_LEVEL_10_0,
  };
  D3D_FEATURE_LEVEL level{};
  HRESULT hr = D3D11CreateDevice(
    nullptr,
    D3D_DRIVER_TYPE_HARDWARE,
    nullptr,
    flags,
    levels,
    ARRAYSIZE(levels),
    D3D11_SDK_VERSION,
    g.device.put(),
    &level,
    g.context.put());
  if (FAILED(hr)) {
    return false;
  }

  com_ptr<IDXGIDevice> dxgiDevice;
  if (FAILED(g.device.as(dxgiDevice))) {
    return false;
  }

  com_ptr<IInspectable> inspectable;
  if (FAILED(CreateDirect3D11DeviceFromDXGIDevice(dxgiDevice.get(), inspectable.put()))) {
    return false;
  }
  g.winrtDevice = inspectable.as<IDirect3DDevice>();
  return createShaders();
}

bool createSwapChain() {
  if (!g.hwnd || !g.device || g.width <= 0 || g.height <= 0) {
    return false;
  }
  g.shaderTexture = nullptr;
  g.shaderView = nullptr;
  if (g.swapChain) {
    return SUCCEEDED(g.swapChain->ResizeBuffers(2, g.width, g.height, DXGI_FORMAT_B8G8R8A8_UNORM, 0));
  }

  com_ptr<IDXGIDevice> dxgiDevice;
  com_ptr<IDXGIAdapter> adapter;
  com_ptr<IDXGIFactory> factory;
  if (FAILED(g.device.as(dxgiDevice)) ||
      FAILED(dxgiDevice->GetAdapter(adapter.put())) ||
      FAILED(adapter->GetParent(__uuidof(IDXGIFactory), factory.put_void()))) {
    return false;
  }

  DXGI_SWAP_CHAIN_DESC desc{};
  desc.BufferDesc.Width = g.width;
  desc.BufferDesc.Height = g.height;
  desc.BufferDesc.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
  desc.BufferDesc.RefreshRate.Numerator = 0;
  desc.BufferDesc.RefreshRate.Denominator = 1;
  desc.SampleDesc.Count = 1;
  desc.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
  desc.BufferCount = 2;
  desc.OutputWindow = g.hwnd;
  desc.Windowed = TRUE;
  desc.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;
  return SUCCEEDED(factory->CreateSwapChain(g.device.get(), &desc, g.swapChain.put()));
}

GraphicsCaptureItem createItemForWindow(HWND hwnd) {
  auto interop = get_activation_factory<GraphicsCaptureItem, IGraphicsCaptureItemInterop>();
  GraphicsCaptureItem item{ nullptr };
  check_hresult(interop->CreateForWindow(
    hwnd,
    guid_of<GraphicsCaptureItem>(),
    put_abi(item)));
  return item;
}

bool ensureShaderTexture(ID3D11Texture2D* frameTexture) {
  D3D11_TEXTURE2D_DESC frameDesc{};
  frameTexture->GetDesc(&frameDesc);
  if (g.shaderTexture) {
    D3D11_TEXTURE2D_DESC existing{};
    g.shaderTexture->GetDesc(&existing);
    if (existing.Width == frameDesc.Width && existing.Height == frameDesc.Height && existing.Format == frameDesc.Format) {
      return true;
    }
  }

  g.shaderView = nullptr;
  g.shaderTexture = nullptr;
  D3D11_TEXTURE2D_DESC desc = frameDesc;
  desc.BindFlags = D3D11_BIND_SHADER_RESOURCE;
  desc.CPUAccessFlags = 0;
  desc.MiscFlags = 0;
  desc.Usage = D3D11_USAGE_DEFAULT;
  if (FAILED(g.device->CreateTexture2D(&desc, nullptr, g.shaderTexture.put()))) {
    return false;
  }

  D3D11_SHADER_RESOURCE_VIEW_DESC srv{};
  srv.Format = desc.Format;
  srv.ViewDimension = D3D11_SRV_DIMENSION_TEXTURE2D;
  srv.Texture2D.MipLevels = 1;
  return SUCCEEDED(g.device->CreateShaderResourceView(g.shaderTexture.get(), &srv, g.shaderView.put()));
}

void renderFrame(ID3D11Texture2D* frameTexture) {
  if (!g.swapChain || !frameTexture || !ensureShaderTexture(frameTexture)) {
    return;
  }

  g.context->CopyResource(g.shaderTexture.get(), frameTexture);

  com_ptr<ID3D11Texture2D> backBuffer;
  if (FAILED(g.swapChain->GetBuffer(0, __uuidof(ID3D11Texture2D), backBuffer.put_void()))) {
    return;
  }
  com_ptr<ID3D11RenderTargetView> rtv;
  if (FAILED(g.device->CreateRenderTargetView(backBuffer.get(), nullptr, rtv.put()))) {
    return;
  }

  D3D11_VIEWPORT viewport{};
  viewport.Width = static_cast<float>(g.width);
  viewport.Height = static_cast<float>(g.height);
  viewport.MinDepth = 0.0f;
  viewport.MaxDepth = 1.0f;

  float clear[4] = { 0.02f, 0.03f, 0.05f, 1.0f };
  ID3D11RenderTargetView* targets[] = { rtv.get() };
  g.context->OMSetRenderTargets(1, targets, nullptr);
  g.context->ClearRenderTargetView(rtv.get(), clear);
  g.context->RSSetViewports(1, &viewport);
  g.context->IASetPrimitiveTopology(D3D11_PRIMITIVE_TOPOLOGY_TRIANGLESTRIP);
  g.context->VSSetShader(g.vertexShader.get(), nullptr, 0);
  g.context->PSSetShader(g.pixelShader.get(), nullptr, 0);
  ID3D11ShaderResourceView* srv = g.shaderView.get();
  ID3D11SamplerState* sampler = g.sampler.get();
  g.context->PSSetShaderResources(0, 1, &srv);
  g.context->PSSetSamplers(0, 1, &sampler);
  g.context->Draw(4, 0);
  ID3D11ShaderResourceView* nullSrv = nullptr;
  g.context->PSSetShaderResources(0, 1, &nullSrv);
  g.swapChain->Present(1, 0);
}

void onFrameArrived(Direct3D11CaptureFramePool const& sender, IInspectable const&) {
  std::lock_guard<std::mutex> lock(g.mu);
  if (!g.running) {
    return;
  }
  auto frame = sender.TryGetNextFrame();
  auto size = frame.ContentSize();
  if (size.Width != g.lastItemSize.Width || size.Height != g.lastItemSize.Height) {
    g.lastItemSize = size;
    g.framePool.Recreate(g.winrtDevice, DirectXPixelFormat::B8G8R8A8UIntNormalized, 2, size);
    return;
  }

  auto access = frame.Surface().as<::Windows::Graphics::DirectX::Direct3D11::IDirect3DDxgiInterfaceAccess>();
  com_ptr<ID3D11Texture2D> texture;
  if (SUCCEEDED(access->GetInterface(__uuidof(ID3D11Texture2D), texture.put_void()))) {
    renderFrame(texture.get());
  }
}

void stopLocked() {
  g.running = false;
  if (g.framePool && g.frameToken.value) {
    g.framePool.FrameArrived(g.frameToken);
    g.frameToken = {};
  }
  if (g.session) {
    g.session.Close();
    g.session = nullptr;
  }
  if (g.framePool) {
    g.framePool.Close();
    g.framePool = nullptr;
  }
  g.item = nullptr;
  g.shaderView = nullptr;
  g.shaderTexture = nullptr;
  if (g.hwnd) {
    ShowWindow(g.hwnd, SW_HIDE);
  }
}

} // namespace

extern "C" int bitstream_wgc_is_supported() {
  try {
    init_apartment(apartment_type::multi_threaded);
    return GraphicsCaptureSession::IsSupported() ? 1 : 0;
  } catch (...) {
    return 0;
  }
}

extern "C" int bitstream_wgc_can_create_item_for_hwnd(void* hwnd) {
  try {
    if (!hwnd || !GraphicsCaptureSession::IsSupported()) {
      return 0;
    }
    init_apartment(apartment_type::multi_threaded);
    auto item = createItemForWindow(static_cast<HWND>(hwnd));
    return item ? 1 : 0;
  } catch (...) {
    return 0;
  }
}

extern "C" int bitstream_wgc_preview_start(void* parent, void* capture, int x, int y, int width, int height) {
  try {
    if (!parent || !capture || width <= 0 || height <= 0 || !GraphicsCaptureSession::IsSupported()) {
      return 0;
    }
    init_apartment(apartment_type::multi_threaded);
    std::lock_guard<std::mutex> lock(g.mu);

    if (!createDevice() || !ensureWindowClass()) {
      return 0;
    }

    HWND parentHwnd = static_cast<HWND>(parent);
    HWND captureHwnd = static_cast<HWND>(capture);
    if (!g.hwnd) {
      g.hwnd = CreateWindowExW(
        0,
        kPreviewClass,
        L"",
        WS_CHILD | WS_VISIBLE | WS_CLIPSIBLINGS,
        x,
        y,
        width,
        height,
        parentHwnd,
        nullptr,
        GetModuleHandleW(nullptr),
        nullptr);
      if (!g.hwnd) {
        return 0;
      }
    }

    g.parent = parentHwnd;
    g.captureHwnd = captureHwnd;
    g.x = x;
    g.y = y;
    g.width = width;
    g.height = height;
    SetParent(g.hwnd, parentHwnd);
    SetWindowPos(g.hwnd, HWND_TOP, x, y, width, height, SWP_SHOWWINDOW);
    ShowWindow(g.hwnd, SW_SHOW);

    if (!createSwapChain()) {
      return 0;
    }

    if (!g.running || !g.item || g.captureHwnd != captureHwnd) {
      stopLocked();
      g.captureHwnd = captureHwnd;
      g.item = createItemForWindow(captureHwnd);
      g.lastItemSize = g.item.Size();
      g.framePool = Direct3D11CaptureFramePool::CreateFreeThreaded(
        g.winrtDevice,
        DirectXPixelFormat::B8G8R8A8UIntNormalized,
        2,
        g.lastItemSize);
      g.session = g.framePool.CreateCaptureSession(g.item);
      try {
        g.session.IsCursorCaptureEnabled(true);
      } catch (...) {
      }
      g.frameToken = g.framePool.FrameArrived(&onFrameArrived);
      g.running = true;
      g.session.StartCapture();
    }
    return 1;
  } catch (...) {
    return 0;
  }
}

extern "C" void bitstream_wgc_preview_move(int x, int y, int width, int height) {
  std::lock_guard<std::mutex> lock(g.mu);
  if (!g.hwnd || width <= 0 || height <= 0) {
    return;
  }
  g.x = x;
  g.y = y;
  g.width = width;
  g.height = height;
  SetWindowPos(g.hwnd, HWND_TOP, x, y, width, height, SWP_SHOWWINDOW);
  createSwapChain();
}

extern "C" void bitstream_wgc_preview_stop() {
  std::lock_guard<std::mutex> lock(g.mu);
  stopLocked();
}
