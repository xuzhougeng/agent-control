import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

enum ChatAttachmentCodecError: LocalizedError {
    case notImage
    case tooLarge
    case encodeFailed

    var errorDescription: String? {
        switch self {
        case .notImage:
            return "Unsupported image format"
        case .tooLarge:
            return "Image is too large after compression"
        case .encodeFailed:
            return "Failed to encode image"
        }
    }
}

enum ChatAttachmentCodec {
    static let maxItemsPerMessage = 3
    static let maxBytes = 900 * 1024
    static let maxEdge: CGFloat = 1800

    static func makeAttachment(name: String, data: Data, mediaTypeHint: String? = nil) throws -> ChatAttachment {
        guard let source = CGImageSourceCreateWithData(data as CFData, nil),
              let image = CGImageSourceCreateImageAtIndex(source, 0, nil) else {
            throw ChatAttachmentCodecError.notImage
        }
        let scaled = try resizeIfNeeded(image)

        if let mediaTypeHint, mediaTypeHint.lowercased() == "image/png",
           let png = encode(image: scaled, uti: UTType.png.identifier, quality: nil),
           png.count <= maxBytes {
            return ChatAttachment(
                name: name.isEmpty ? "image.png" : name,
                mediaType: "image/png",
                base64Data: png.base64EncodedString(),
                rawData: png
            )
        }

        if let png = encode(image: scaled, uti: UTType.png.identifier, quality: nil), png.count <= maxBytes {
            return ChatAttachment(
                name: name.isEmpty ? "image.png" : name,
                mediaType: "image/png",
                base64Data: png.base64EncodedString(),
                rawData: png
            )
        }

        for quality in [0.9, 0.82, 0.72, 0.6, 0.48] {
            if let jpeg = encode(image: scaled, uti: UTType.jpeg.identifier, quality: quality),
               jpeg.count <= maxBytes {
                return ChatAttachment(
                    name: name.isEmpty ? "image.jpg" : name,
                    mediaType: "image/jpeg",
                    base64Data: jpeg.base64EncodedString(),
                    rawData: jpeg
                )
            }
        }
        throw ChatAttachmentCodecError.tooLarge
    }

    private static func resizeIfNeeded(_ image: CGImage) throws -> CGImage {
        let width = CGFloat(image.width)
        let height = CGFloat(image.height)
        let edge = max(width, height)
        guard edge > 0 else { throw ChatAttachmentCodecError.notImage }
        let scale = min(1, maxEdge / edge)
        let targetW = max(1, Int((width * scale).rounded()))
        let targetH = max(1, Int((height * scale).rounded()))
        guard targetW != image.width || targetH != image.height else { return image }

        let colorSpace = CGColorSpaceCreateDeviceRGB()
        guard let ctx = CGContext(
            data: nil,
            width: targetW,
            height: targetH,
            bitsPerComponent: 8,
            bytesPerRow: 0,
            space: colorSpace,
            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        ) else {
            throw ChatAttachmentCodecError.encodeFailed
        }
        ctx.interpolationQuality = .high
        ctx.draw(image, in: CGRect(x: 0, y: 0, width: targetW, height: targetH))
        guard let scaled = ctx.makeImage() else {
            throw ChatAttachmentCodecError.encodeFailed
        }
        return scaled
    }

    private static func encode(image: CGImage, uti: String, quality: CGFloat?) -> Data? {
        let out = NSMutableData()
        guard let dest = CGImageDestinationCreateWithData(out, uti as CFString, 1, nil) else { return nil }
        var options: [CFString: Any] = [:]
        if let quality {
            options[kCGImageDestinationLossyCompressionQuality] = quality
        }
        CGImageDestinationAddImage(dest, image, options as CFDictionary)
        guard CGImageDestinationFinalize(dest) else { return nil }
        return out as Data
    }
}
