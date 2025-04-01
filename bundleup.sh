#!/usr/bin/env bash

# Description:
# This script searches for all .go files and go.mod file within the current directory and its subdirectories,
# excluding the 'bundle' directory itself, the '.git', '.idea', and '.vscode' directories.
# It then empties the 'bundle' directory if it exists (or creates it if it doesn't),
# copies all found .go files and go.mod into the 'bundle' directory,
# and creates a text file with the folder structure and file listing.

# Exit immediately if a command exits with a non-zero status
set -e

# Define the bundle directory name
BUNDLE_DIR="bundle"
# Define the structure file name
STRUCTURE_FILE="$BUNDLE_DIR/files_folders_structure.txt"

# Function to display messages
echo_msg() {
    echo "[$(date +"%Y-%m-%d %H:%M:%S")] $1"
}

# Step 1: Remove all contents of the bundle directory if it exists, else create it
if [ -d "$BUNDLE_DIR" ]; then
    echo_msg "Emptying the '$BUNDLE_DIR' directory..."
    rm -rf "${BUNDLE_DIR:?}/"*
else
    echo_msg "Creating the '$BUNDLE_DIR' directory..."
    mkdir "$BUNDLE_DIR"
fi

# Step 2: Find all .go files excluding specific directories
echo_msg "Searching for .go files..."
GO_FILES=$(find . -path "./$BUNDLE_DIR" -prune -o -path "./.git" -prune -o -path "./.idea" -prune -o -path "./.vscode" -prune -o -type f -name "*.go" -print)

# Check if any .go files were found
if [ -z "$GO_FILES" ]; then
    echo_msg "No .go files found."
else
    # Step 3: Copy each .go file to the bundle directory
    echo_msg "Copying .go files to '$BUNDLE_DIR'..."
    while IFS= read -r file; do
        cp "$file" "$BUNDLE_DIR/"
        echo_msg "Copied: $file"
    done <<< "$GO_FILES"
fi

# Step 4: Find and copy go.mod file if it exists
echo_msg "Searching for go.mod file..."
if [ -f "go.mod" ]; then
    cp "go.mod" "$BUNDLE_DIR/"
    echo_msg "Copied: go.mod"
else
    echo_msg "No go.mod file found."
fi

# Step 5: Create a text file with folder structure and file listing
echo_msg "Creating folder structure file..."
{
    echo "Folder Structure:"
    echo "================="
    find . -path "./$BUNDLE_DIR" -prune -o -path "./.git" -prune -o -path "./.idea" -prune -o -path "./.vscode" -prune -o -type d -print | sort

    echo -e "\nFile Listing:"
    echo "============="
    find . -path "./$BUNDLE_DIR" -prune -o -path "./.git" -prune -o -path "./.idea" -prune -o -path "./.vscode" -prune -o -type f -print | sort

    echo -e "\nBundled Files:"
    echo "=============="
    ls -la "$BUNDLE_DIR"
} > "$STRUCTURE_FILE"

echo_msg "All files have been bundled successfully."
echo_msg "Folder structure and file listing saved to $STRUCTURE_FILE"