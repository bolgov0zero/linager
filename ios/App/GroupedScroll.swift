import SwiftUI

// The look of an inset grouped list on a ScrollView. A List cannot keep a header under the bar once it has scrolled there (in a grouped
// style the headers scroll away), and a ScrollView can (`pinnedViews`), so the screens with a switch that stays under the bar are drawn here
// in the same look: rounded cards on the grouped background, lines between rows, the system margins.

/// The side margin of the cards and of the switch (the margin of the search field of the bar).
let groupMargin: CGFloat = 16

/// A card of rows.
struct GroupCard<Content: View>: View {
    @ViewBuilder let content: () -> Content

    var body: some View {
        VStack(spacing: 0, content: content)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: cardRadius, style: .continuous))
            .padding(.horizontal, groupMargin)
    }
}

/// A row of a card: the room around the content and a line above it (not above the first row), starting `lineInset` from the edge of the card.
struct GroupRow<Content: View>: View {
    var first = false
    var lineInset: CGFloat = 16
    var minHeight: CGFloat = rowHeight
    @ViewBuilder let content: () -> Content

    var body: some View {
        content()
            .padding(.leading, 16)
            .padding(.trailing, 16)
            .frame(minHeight: minHeight)
            .frame(maxWidth: .infinity, alignment: .leading)
            .overlay(alignment: .top) {
                if !first { Divider().padding(.leading, lineInset) }
            }
    }
}

/// The sizes of the application, the same on every screen (measured on the system apps of iOS 26): a card is rounded by `cardRadius`, a small
/// box (a field with a code, a banner) by `boxRadius`, a control (the search field, the switch) is `controlHeight` tall, a row at least `rowHeight`.
let cardRadius: CGFloat = 26
let boxRadius: CGFloat = 16
let controlHeight: CGFloat = 44
let rowHeight: CGFloat = 48

/// A screen of a scrolling list with a switch in it: what is `above` the switch scrolls away, the switch scrolls up with the list and, reaching
/// the bar, stays under it (a pinned header of the list), and `below` is the list. The switch has no backing.
///
/// (A bar docked with `safeAreaBar` would have the blur of the header behind it, but with the large title that folds it made the head jerk:
/// the bar and the title both change the top inset of the list while it scrolls.)
struct PinnedSwitchScroll<Above: View, Bar: View, Below: View>: View {
    let above: Above
    let bar: Bar
    let below: Below
    let refresh: () async -> Void

    init(@ViewBuilder above: () -> Above, @ViewBuilder bar: () -> Bar, @ViewBuilder below: () -> Below, refresh: @escaping () async -> Void) {
        self.above = above()
        self.bar = bar()
        self.below = below()
        self.refresh = refresh
    }

    var body: some View {
        ScrollView {
            LazyVStack(spacing: 0, pinnedViews: [.sectionHeaders]) {
                above
                Section {
                    below
                } header: {
                    bar
                        .padding(.horizontal, groupMargin)
                        .padding(.vertical, 8)
                        .frame(maxWidth: .infinity)
                }
            }
        }
        // a soft edge under the bar: no hard line (a light border) between the bar and the list
        .scrollEdgeEffectStyle(.soft, for: .top)
        .background(Color(.systemGroupedBackground))
        .scrollDismissesKeyboard(.interactively)
        .refreshable { await refresh() }
    }
}

/// A switch of a few kinds made as the bottom bar of the system is: a capsule of glass (the track) and in it a drop of glass, the chosen one. When
/// the choice moves the drop flows from one place to the next (the glass of the places is one container, and the drop has one identity), and
/// while a finger is on the bar the whole bar rises a little and the drop follows the finger along the track. 44 pt tall, as the field of search.
struct GlassSegmented<Tag: Hashable>: View {
    let items: [(tag: Tag, title: LocalizedStringKey)]
    @Binding var selection: Tag
    @Namespace private var drop
    @GestureState private var pressed = false
    private let inset: CGFloat = 3

    private static var spring: Animation { .spring(response: 0.42, dampingFraction: 0.78) }

    var body: some View {
        GeometryReader { geo in
            let count = max(1, items.count)
            let width = (geo.size.width - inset * 2) / CGFloat(count)
            ZStack {
                // the track
                GlassEffectContainer {
                    Color.clear.glassEffect(.regular, in: Capsule())
                }
                // the drop: one place for each kind, the glass is only where the choice is, so moving the choice makes it flow
                GlassEffectContainer(spacing: 24) {
                    HStack(spacing: 0) {
                        ForEach(items, id: \.tag) { item in
                            ZStack {
                                if item.tag == selection {
                                    Color.clear
                                        .frame(maxWidth: .infinity)
                                        .frame(height: controlHeight - inset * 2)
                                        .glassEffect(.regular.interactive(), in: Capsule())
                                        .glassEffectID("drop", in: drop)
                                }
                            }
                            .frame(maxWidth: .infinity)
                        }
                    }
                    .padding(.horizontal, inset)
                }
                // the names
                HStack(spacing: 0) {
                    ForEach(items, id: \.tag) { item in
                        Text(item.title)
                            .font(.subheadline.weight(item.tag == selection ? .semibold : .medium))
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)
                            .frame(maxWidth: .infinity)
                    }
                }
                .padding(.horizontal, inset)
                .allowsHitTesting(false)
            }
            .frame(height: controlHeight)
            .contentShape(Capsule())
            // the bar rises a little while it is touched
            .scaleEffect(pressed ? 1.04 : 1)
            .animation(Self.spring, value: pressed)
            // a tap and a drag are one gesture: the choice follows the finger along the track
            .gesture(
                DragGesture(minimumDistance: 0)
                    .updating($pressed) { _, state, _ in state = true }
                    .onChanged { v in
                        let i = max(0, min(count - 1, Int(((v.location.x - inset) / width).rounded(.down))))
                        if items[i].tag != selection { withAnimation(Self.spring) { selection = items[i].tag } }
                    }
            )
        }
        .frame(height: controlHeight)
        .accessibilityElement(children: .ignore)
        .accessibilityRepresentation {
            Picker("", selection: $selection) {
                ForEach(items, id: \.tag) { Text($0.title).tag($0.tag) }
            }
            .pickerStyle(.segmented)
        }
    }
}

/// The frame of a row of a card as a modifier: for a row that already has its own content padding (the rows here pad themselves by depth).
struct RowFrame: ViewModifier {
    var first = false
    var lineInset: CGFloat = 16

    func body(content: Content) -> some View {
        GroupRow(first: first, lineInset: lineInset, minHeight: rowHeight) { content }
    }
}
