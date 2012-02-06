import os
from book_file_processors import BookFileProcessorFactory, UnsupportedBookFileType
from db.dao import BooksDao


class BookFileImporter(object):
    dao = None
    book_processor_factory_cls = None

    @classmethod
    def get_instance(cls):
        instance = cls()
        instance.dao = BooksDao.get_instance()
        instance.book_processor_factory_cls = BookFileProcessorFactory
        return instance

    def try_to_import_file_if_not_already_imported(self, path):
        path = unicode(path, encoding='utf-8')
        if self.is_file_already_imported(path):
            return
        try:
            book = self.get_book_file_processor(path).get_book()
        except UnsupportedBookFileType:
            return
        print '+ %s' % path
        self.save_book(book)

    def save_book(self, book):
        self.dao.save_book(book)

    def is_file_already_imported(self, path):
        return self.dao.is_book_with_path_already_exists(path)

    def get_book_file_processor(self, path):
        return self.book_processor_factory_cls(path).get_book_file_processor()
