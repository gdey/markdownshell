import MdshKit
import SwiftUI

/// CardAnchors collects each block card's bounds, by block index, so
/// the dependency lines can be drawn between them.
struct CardAnchors: PreferenceKey {
    static let defaultValue: [Int: Anchor<CGRect>] = [:]
    static func reduce(value: inout [Int: Anchor<CGRect>], nextValue: () -> [Int: Anchor<CGRect>]) {
        value.merge(nextValue()) { $1 }
    }
}

/// DepEdge is one dependency: block from depends on block to, as the
/// k-th (from 0) in its depends=.
struct DepEdge: Hashable {
    var from: Int
    var to: Int
    var k: Int

    /// edges works out the document's dependencies by block index. A
    /// textbox or a choice never runs, so naming one gets no line.
    static func of(_ cells: [Cell]) -> [DepEdge] {
        var index: [String: Int] = [:]
        for c in cells where c.isBlock && !c.blockID.isEmpty && c.textbox == nil && c.choice == nil {
            index[c.blockID] = c.block
        }
        var out: [DepEdge] = []
        for c in cells where c.isBlock {
            for (k, id) in (c.depends ?? []).enumerated() {
                if let to = index[id], to != c.block { out.append(DepEdge(from: c.block, to: to, k: k)) }
            }
        }
        return out
    }

    /// color is the k-th dependency's color, mdsh-gui's --dep-1 to
    /// --dep-4, and gray past them.
    static func color(_ k: Int, dark: Bool) -> Color {
        let light = ["#8250df", "#bf3989", "#1b7c83", "#bc4c00", "#8c959f"]
        let night = ["#a475f9", "#f778ba", "#39c5cf", "#f0883e", "#6e7781"]
        return Color(nsColor: Theme.nsColor((dark ? night : light)[min(k, 4)]))
    }
}

/// DepLines draws the lines between a block and the blocks its depends=
/// names, in the margin beside the cards, as mdsh-gui paints them behind
/// the page: from the dependent block's head row out to a lane, up to
/// the block it depends on, and in to a dot at its bottom anchor. Each
/// line takes a lane of its own, out from the cards, so lines over the
/// same rows don't merge; the selected block's lines are lit.
///
/// Editing them by dragging, as mdsh-gui does, isn't here yet: Depends
/// On (d) edits them.
struct DepLines: View {
    var edges: [DepEdge]
    var cards: [Int: CGRect]
    var lit: Int?
    var theme: Theme

    private let laneGap: CGFloat = 14
    private let laneStep: CGFloat = 10
    private let corner: CGFloat = 6

    private struct Line {
        var edge: DepEdge
        var y1: CGFloat
        var y2: CGFloat
        var lane = 0
        var fromShown = true
        var toShown = true
    }

    var body: some View {
        Canvas { ctx, size in
            guard let cardLeft = cards.values.map(\.minX).min(),
                  let cardRight = cards.values.map(\.maxX).max()
            else { return }
            // The lazy page lays out only the cards near the screen. A
            // line to a card that isn't laid out runs off past the ones
            // that are, the way it goes: up to a block above, down from a
            // block below.
            let top = (cards.values.map(\.minY).min() ?? 0) - 40
            let bottom = (cards.values.map(\.maxY).max() ?? 0) + 40
            var lines: [Line] = []
            for e in edges {
                let a = cards[e.from], b = cards[e.to]
                guard a != nil || b != nil else { continue }
                // The top anchor is on the head row; the bottom one near
                // the card's foot.
                let y1 = a.map { $0.minY + min($0.height / 2, 14) } ?? (e.to < e.from ? top : bottom)
                let y2 = b.map { $0.maxY - min($0.height / 2, 12) } ?? (e.to < e.from ? top : bottom)
                var l = Line(edge: e, y1: y1, y2: y2)
                l.fromShown = a != nil
                l.toShown = b != nil
                lines.append(l)
            }
            // Lanes: the shortest lines nearest the cards, each in the
            // first lane free over its rows.
            var lanes: [[Line]] = []
            for var l in lines.sorted(by: { abs($0.y1 - $0.y2) < abs($1.y1 - $1.y2) }) {
                let lo = min(l.y1, l.y2), hi = max(l.y1, l.y2)
                var i = 0
                while i < lanes.count, lanes[i].contains(where: { min($0.y1, $0.y2) < hi && lo < max($0.y1, $0.y2) }) { i += 1 }
                if i == lanes.count { lanes.append([]) }
                l.lane = i
                lanes[i].append(l)
            }
            // The left margin when the lanes fit there, else the right,
            // else whichever has more room, with the lanes that don't fit
            // sharing the last one that does.
            let need = laneGap + laneStep * CGFloat(max(lanes.count - 1, 0)) + 4
            let roomLeft = cardLeft, roomRight = size.width - cardRight
            let onLeft = roomLeft >= need || (roomRight < need && roomLeft >= roomRight)
            let room = onLeft ? roomLeft : roomRight
            let fit = max(0, Int((room - laneGap - 4) / laneStep))
            let dir: CGFloat = onLeft ? -1 : 1
            let edgeX = onLeft ? cardLeft : cardRight
            // Lit lines draw last, over the others.
            for l in lanes.flatMap({ $0 }).sorted(by: { ($0.edge.from == lit ? 1 : 0) < ($1.edge.from == lit ? 1 : 0) }) {
                let x = edgeX + dir * (laneGap + laneStep * CGFloat(min(l.lane, fit)))
                let on = l.edge.from == lit
                let color = DepEdge.color(l.edge.k, dark: theme.dark).opacity(lit == nil || on ? 0.9 : 0.35)
                ctx.stroke(elbow(edgeX, x, l.y1, l.y2), with: .color(color), lineWidth: on ? 2 : 1.25)
                // A ring at the top anchor, where the line leaves, and a dot
                // at the bottom anchor, where it arrives, both just out
                // from the card's edge.
                let out = dir * 5
                if l.toShown {
                    ctx.fill(Path(ellipseIn: CGRect(x: edgeX + out - 3.5, y: l.y2 - 3.5, width: 7, height: 7)), with: .color(color))
                }
                if l.fromShown {
                    ctx.stroke(Path(ellipseIn: CGRect(x: edgeX + out - 4, y: l.y1 - 4, width: 8, height: 8)), with: .color(color), lineWidth: 1.5)
                }
            }
        }
        .allowsHitTesting(false)
        .accessibilityHidden(true)
    }

    /// elbow is the line from the card edge at y1 out to lane x, along
    /// it to y2, and back in to the edge, with rounded corners.
    private func elbow(_ edge: CGFloat, _ x: CGFloat, _ y1: CGFloat, _ y2: CGFloat) -> Path {
        var p = Path()
        let r = min(corner, abs(y2 - y1) / 2, abs(x - edge))
        let sy: CGFloat = y2 < y1 ? -1 : 1
        let sx: CGFloat = x < edge ? -1 : 1
        p.move(to: CGPoint(x: edge, y: y1))
        p.addLine(to: CGPoint(x: x - sx * r, y: y1))
        p.addQuadCurve(to: CGPoint(x: x, y: y1 + sy * r), control: CGPoint(x: x, y: y1))
        p.addLine(to: CGPoint(x: x, y: y2 - sy * r))
        p.addQuadCurve(to: CGPoint(x: x - sx * r, y: y2), control: CGPoint(x: x, y: y2))
        p.addLine(to: CGPoint(x: edge, y: y2))
        return p
    }
}

/// DepRings outlines a card the selected block depends on, a ring in
/// each of its lines' colors, as mdsh-gui marks them.
struct DepRings: ViewModifier {
    var rings: [Int]  // the k of each line leading here
    var theme: Theme

    func body(content: Content) -> some View {
        content.overlay {
            ForEach(Array(rings.enumerated()), id: \.offset) { i, k in
                RoundedRectangle(cornerRadius: theme.look == .plain ? 8 + CGFloat(3 + 2 * i) : 0)
                    .stroke(DepEdge.color(k, dark: theme.dark), lineWidth: 2)
                    .padding(-CGFloat(3 + 2 * i))
                    .allowsHitTesting(false)
            }
        }
    }
}
