import os

from library.book_file_processors import BookFileProcessorFactory
from library.book_file_processors import UnsupportedBookFileType


class DirectoryImporter(object):
    def __init__(self, path):
        self.path = path

    def do_import(self):
        for path in self.get_all_filenames_iter():
            self.try_to_import_file_if_not_already_imported(path)

    def try_to_import_file_if_not_already_imported(self, path):
        BookFileImporter(path).try_to_import_file_if_not_already_imported()

    def get_all_filenames_iter(self):
        for path, dirs, files in os.walk(self.path):
            for file in files:
                yield os.path.join(path, file)


class BookFileImporter(object):
    db = None
    BookFileProcessorFactory = BookFileProcessorFactory

    def __init__(self, path):
        self.path = path

    def try_to_import_file_if_not_already_imported(self):
        if self.is_file_already_imported():
            return
        try:
            book = self.get_book_file_processor().get_book()
        except UnsupportedBookFileType:
            return
        self.save_book(book)

    def save_book(self, book):
        self.db.save_book(book)

    def is_file_already_imported(self):
        return self.db.book_with_path_already_exists(self.path)

    def get_book_file_processor(self):
        return self.BookFileProcessorFactory(self.path).get_book_file_processor()
