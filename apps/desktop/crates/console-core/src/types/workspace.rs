//! The workspace pane-tree model.
//!
//! Mirrors the desktop app's workspace: a recursive tree of **split** nodes
//! (resizable side-by-side panes) and **leaf** nodes (a tab strip plus the
//! active tab's content). Pure data — no gpui dependencies — so the tree can
//! be persisted and rebuilt across launches.

use serde::{Deserialize, Serialize};

/// One tab inside a workspace leaf. Each tab names a view: a chat session, a
/// terminal, a file, or a diff.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum WorkspaceTabConfig {
    #[serde(rename = "chat")]
    Chat {
        #[serde(rename = "sessionId")]
        session_id: String,
        title: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        project_id: Option<String>,
    },
    #[serde(rename = "terminal")]
    Terminal {
        #[serde(rename = "terminalId")]
        terminal_id: String,
        title: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        project_id: Option<String>,
    },
    #[serde(rename = "file")]
    File {
        path: String,
        title: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        project_id: Option<String>,
    },
    #[serde(rename = "diff")]
    Diff {
        path: String,
        title: String,
        #[serde(default, skip_serializing_if = "Option::is_none")]
        project_id: Option<String>,
    },
}

impl WorkspaceTabConfig {
    /// Stable identity used to compare and select tabs (`chat:<id>`, …).
    pub fn id(&self) -> String {
        match self {
            Self::Chat { session_id, .. } => format!("chat:{session_id}"),
            Self::Terminal { terminal_id, .. } => format!("term:{terminal_id}"),
            Self::File { path, .. } => format!("file:{path}"),
            Self::Diff { path, .. } => format!("diff:{path}"),
        }
    }

    pub fn title(&self) -> &str {
        match self {
            Self::Chat { title, .. }
            | Self::Terminal { title, .. }
            | Self::File { title, .. }
            | Self::Diff { title, .. } => title,
        }
    }

    pub fn set_title(&mut self, title: impl Into<String>) {
        let title = title.into();
        match self {
            Self::Chat { title: t, .. }
            | Self::Terminal { title: t, .. }
            | Self::File { title: t, .. }
            | Self::Diff { title: t, .. } => *t = title,
        }
    }

    pub fn project_id(&self) -> Option<&str> {
        match self {
            Self::Chat { project_id, .. }
            | Self::Terminal { project_id, .. }
            | Self::File { project_id, .. }
            | Self::Diff { project_id, .. } => project_id.as_deref(),
        }
    }
}

/// A leaf pane: a tab strip plus the currently active tab.
#[derive(Clone, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct LeafPaneNode {
    pub id: String,
    pub tabs: Vec<WorkspaceTabConfig>,
    pub active_tab_id: Option<String>,
}

/// A split between two panes, with proportional sizes in percent.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SplitPaneNode {
    pub id: String,
    /// `horizontal` = side-by-side columns; `vertical` = stacked rows.
    pub direction: SplitDirection,
    /// Percentage sizes of the two children.
    pub sizes: [f32; 2],
    pub children: [Box<WorkspaceNode>; 2],
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum SplitDirection {
    Horizontal,
    Vertical,
}

/// The workspace layout: a tree of leaves and splits.
#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum WorkspaceNode {
    #[serde(rename = "leaf")]
    Leaf(LeafPaneNode),
    #[serde(rename = "split")]
    Split(SplitPaneNode),
}

impl WorkspaceNode {
    pub fn leaf(id: impl Into<String>) -> Self {
        Self::Leaf(LeafPaneNode {
            id: id.into(),
            tabs: Vec::new(),
            active_tab_id: None,
        })
    }

    /// The id of this node, whether leaf or split.
    pub fn id(&self) -> &str {
        match self {
            Self::Leaf(node) => &node.id,
            Self::Split(node) => &node.id,
        }
    }

    /// All leaf panes under this node, in layout order.
    pub fn leaves(&self) -> Vec<&LeafPaneNode> {
        let mut out = Vec::new();
        self.collect_leaves(&mut out);
        out
    }

    fn collect_leaves<'a>(&'a self, out: &mut Vec<&'a LeafPaneNode>) {
        match self {
            Self::Leaf(node) => out.push(node),
            Self::Split(node) => {
                node.children[0].collect_leaves(out);
                node.children[1].collect_leaves(out);
            }
        }
    }

    /// All leaf panes under this node, mutably.
    pub fn leaves_mut(&mut self) -> Vec<&mut LeafPaneNode> {
        let mut out = Vec::new();
        self.collect_leaves_mut(&mut out);
        out
    }

    fn collect_leaves_mut<'a>(&'a mut self, out: &mut Vec<&'a mut LeafPaneNode>) {
        match self {
            Self::Leaf(node) => out.push(node),
            Self::Split(node) => {
                let (left, right) = node.children.as_mut().split_at_mut(1);
                left[0].collect_leaves_mut(out);
                right[0].collect_leaves_mut(out);
            }
        }
    }

    /// Find the leaf with the given id, mutably.
    pub fn leaf_mut(&mut self, id: &str) -> Option<&mut LeafPaneNode> {
        match self {
            Self::Leaf(node) => (node.id == id).then_some(node),
            Self::Split(node) => {
                let (left, right) = node.children.as_mut().split_at_mut(1);
                left[0].leaf_mut(id).or_else(|| right[0].leaf_mut(id))
            }
        }
    }

    /// The first leaf in layout order (the default target when none is active).
    pub fn first_leaf(&self) -> Option<&LeafPaneNode> {
        self.leaves().into_iter().next()
    }
}

/// Construct a two-pane split between `left` and `right`, sized 50/50.
pub fn split_node(
    id: impl Into<String>,
    direction: SplitDirection,
    left: WorkspaceNode,
    right: WorkspaceNode,
) -> WorkspaceNode {
    WorkspaceNode::Split(SplitPaneNode {
        id: id.into(),
        direction,
        sizes: [50.0, 50.0],
        children: [Box::new(left), Box::new(right)],
    })
}
