//go:build wgc

#include <unknwn.h>
#include <winrt/base.h>
#include <winrt/Windows.Graphics.Capture.h>
#include <windows.graphics.capture.interop.h>

extern "C" int bitstream_wgc_is_supported() {
  try {
    winrt::init_apartment(winrt::apartment_type::multi_threaded);
    return winrt::Windows::Graphics::Capture::GraphicsCaptureSession::IsSupported() ? 1 : 0;
  } catch (...) {
    return 0;
  }
}

extern "C" int bitstream_wgc_can_create_item_for_hwnd(void* hwnd) {
  try {
    if (!hwnd || !winrt::Windows::Graphics::Capture::GraphicsCaptureSession::IsSupported()) {
      return 0;
    }
    winrt::init_apartment(winrt::apartment_type::multi_threaded);
    auto interop = winrt::get_activation_factory<
      winrt::Windows::Graphics::Capture::GraphicsCaptureItem,
      IGraphicsCaptureItemInterop>();
    winrt::Windows::Graphics::Capture::GraphicsCaptureItem item{ nullptr };
    winrt::check_hresult(interop->CreateForWindow(
      static_cast<HWND>(hwnd),
      winrt::guid_of<winrt::Windows::Graphics::Capture::GraphicsCaptureItem>(),
      winrt::put_abi(item)));
    return item ? 1 : 0;
  } catch (...) {
    return 0;
  }
}
