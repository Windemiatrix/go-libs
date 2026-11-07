// Package photokit reads and changes the system Photos library through
// PhotoKit: a port of photokit.py of osxphotos. Library.Import, which adds
// files to the library, is an extension: photokit.py does not import. So are
// Library.Collections and the methods that change folders and albums:
// osxphotos reads them from the database and changes them through AppleScript.
// So is Library.Delete, which moves assets to Recently Deleted: photokit.py
// does not delete. So is Library.Update, which changes the date, the flags and
// the location of an asset: photokit.py only reads them. So are
// Library.CloudIdentifiers and Library.LocalIdentifiers, which map the
// LocalIdentifier of an asset to its cloud identifier and back: photokit.py
// has no cloud identifiers.
package photokit
